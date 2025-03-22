package main

import (
	"crypto/tls"
	"log"
	"net"
	"time"

	quic "github.com/quic-go/quic-go"
	cli "github.com/urfave/cli/v2"
	"golang.org/x/net/context"
)

func client(c *cli.Context) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	config := &tls.Config{
		InsecureSkipVerify: true,
		NextProtos:         []string{"quicssh"},
	}

	udpAddr, err := net.ResolveUDPAddr("udp", c.String("addr"))
	if err != nil {
		return er(err)
	}
	srcAddr, err := net.ResolveUDPAddr("udp", c.String("localaddr"))
	if err != nil {
		return er(err)
	}

	log.Printf("Dialing %q->%q...", srcAddr.String(), udpAddr.String())
	conn, err := net.ListenUDP("udp", srcAddr)
	if err != nil {
		return er(err)
	}
	quicConfig := &quic.Config{MaxIdleTimeout: 10 * time.Second, KeepAlivePeriod: 5 * time.Second}
	session, err := quic.Dial(ctx, conn, udpAddr, config, quicConfig)
	if err != nil {
		return er(err)
	}
	defer func() {
		if err := session.CloseWithError(0, "close"); err != nil {
			log.Printf("session close error: %v", err)
		}
	}()

	log.Printf("Opening stream sync...")
	stream, err := session.OpenStreamSync(ctx)
	if err != nil {
		return er(err)
	}
	defer stream.Close()

	log.Printf("Piping stream with QUIC...")
	c1 := readAndWrite(ctx, stream, c.App.Writer) // App.Writer is stdout
	c2 := readAndWrite(ctx, c.App.Reader, stream) // App.Reader is stdin
	select {
	case err = <-c1:
	case err = <-c2:
	}
	if err != nil {
		return err
	}
	return nil
}
