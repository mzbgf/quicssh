package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"strings"
	"time"

	quic "github.com/quic-go/quic-go"
	cli "github.com/urfave/cli/v3"
)

// resolveTxtRecord 解析TXT记录获取IP和端口
func resolveTxtRecord(ctx context.Context, domain string) (string, error) {
	// 移除可能存在的端口号
	domainParts := strings.Split(domain, ":")
	cleanDomain := domainParts[0]
	
	records, err := net.LookupTXT(cleanDomain)
	if err != nil {
		return "", err
	}
	
	if len(records) == 0 {
		return "", fmt.Errorf("no TXT records found for domain: %s", cleanDomain)
	}
	
	// 使用第一条TXT记录，格式应为"ip:port"
	return records[0], nil
}

func client(ctx context.Context, cmd *cli.Command) error {
	ctx = withLabel(ctx, "client")

	config := &tls.Config{
		InsecureSkipVerify: true,
		NextProtos:         []string{"quicssh"},
	}

	addr := cmd.String("addr")
	
	// 检查是否是txt://格式
	if strings.HasPrefix(addr, "txt://") {
		// 提取域名部分，可能包含端口号
		domain := strings.TrimPrefix(addr, "txt://")
		logf(ctx, "解析域名TXT记录: %s", domain)
		
		result, err := resolveTxtRecord(ctx, domain)
		if err != nil {
			return er(ctx, err)
		}
		
		logf(ctx, "TXT记录解析结果: %s", result)
		addr = result
	}

	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return er(ctx, err)
	}
	srcAddr, err := net.ResolveUDPAddr("udp", cmd.String("localaddr"))
	if err != nil {
		return er(ctx, err)
	}

	logf(ctx, "Dialing %q->%q...", srcAddr.String(), udpAddr.String())
	conn, err := net.ListenUDP("udp", srcAddr)
	if err != nil {
		return er(ctx, err)
	}
	quicConfig := &quic.Config{MaxIdleTimeout: 10 * time.Second, KeepAlivePeriod: 5 * time.Second}
	session, err := quic.Dial(ctx, conn, udpAddr, config, quicConfig)
	if err != nil {
		return er(ctx, err)
	}
	defer func() {
		if err := session.CloseWithError(0, "close"); err != nil {
			logf(ctx, "session close error: %v", err)
		}
	}()

	logf(ctx, "Opening stream sync...")
	stream, err := session.OpenStreamSync(ctx)
	if err != nil {
		return er(ctx, err)
	}
	defer stream.Close()

	logf(ctx, "Piping stream with QUIC...")
	c1 := readAndWrite(withLabel(ctx, "stdout"), stream, cmd.Root().Writer) // App.Writer is stdout
	c2 := readAndWrite(withLabel(ctx, "stdin"), cmd.Root().Reader, stream)  // App.Reader is stdin
	select {
	case err = <-c1:
	case err = <-c2:
	}
	if err != nil {
		return err
	}
	return nil
}
