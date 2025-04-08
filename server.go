package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"io"
	"math/big"
	"net"
	"time"

	quic "github.com/quic-go/quic-go"
	cli "github.com/urfave/cli/v3"
)

func server(ctx context.Context, cmd *cli.Command) error {
	ctx = withLabel(ctx, "server")

	// generate TLS certificate
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return er(ctx, err)
	}
	template := x509.Certificate{SerialNumber: big.NewInt(1)}
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		return er(ctx, err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return er(ctx, err)
	}
	config := &tls.Config{
		Certificates: []tls.Certificate{tlsCert},
		NextProtos:   []string{"quicssh"},
	}

	raddr, err := net.ResolveTCPAddr("tcp", cmd.String("sshdaddr"))
	if err != nil {
		return er(ctx, err)
	}

	// configure listener
	listener, err := quic.ListenAddr(cmd.String("bind"), config, nil)
	if err != nil {
		return er(ctx, err)
	}
	defer listener.Close()
	logf(ctx, "Listening at %q... (sshd addr: %q)", cmd.String("bind"), cmd.String("sshdaddr"))

	for {
		logf(ctx, "Accepting connection...")
		session, err := listener.Accept(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return er(ctx, err)
			}
			logf(ctx, "Listener error (sleeping one second): %v", err)
			time.Sleep(time.Second)
			continue
		}

		go serverSessionHandler(ctx, session, raddr)
	}
}

func serverSessionHandler(ctx context.Context, session quic.Connection, raddr *net.TCPAddr) {
	logf(ctx, "Handling session...")
	defer func() {
		if err := session.CloseWithError(0, "close"); err != nil {
			logf(ctx, "Session close error: %v", err)
		}
	}()
	for {
		stream, err := session.AcceptStream(ctx)
		if err != nil {
			logf(ctx, "Session error: %v", err)
			break
		}
		go serverStreamHandler(ctx, stream, raddr)
	}
}

func serverStreamHandler(ctx context.Context, conn io.ReadWriteCloser, raddr *net.TCPAddr) {
	logf(ctx, "Handling stream...")
	defer conn.Close()

	rConn, err := net.DialTCP("tcp", nil, raddr)
	if err != nil {
		logf(ctx, "Dial error: %v", err)
		return
	}
	defer rConn.Close()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	c1 := readAndWrite(withLabel(ctx, "toSSHD"), conn, rConn)
	c2 := readAndWrite(withLabel(ctx, "fromSSHD"), rConn, conn)
	select {
	case err = <-c1:
	case err = <-c2:
	}
	if err != nil {
		logf(ctx, "readAndWrite error: %v", err)
		return
	}
	logf(ctx, "Piping finished")
}
