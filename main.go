package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"os"
	"path"
	"runtime"
	"runtime/debug"

	cli "github.com/urfave/cli/v2"
	"golang.org/x/net/context"
)

func main() {
	build, _ := debug.ReadBuildInfo()
	app := &cli.App{
		Version: build.Main.Version,
		Usage:   "Client and server parts to proxy SSH (TCP) over UDP using QUIC transport",
		Commands: []*cli.Command{
			{
				Name: "server",
				Flags: []cli.Flag{
					&cli.StringFlag{Name: "bind", Value: "localhost:4242", Usage: "bind address"},
					&cli.StringFlag{Name: "sshdaddr", Value: "localhost:22", Usage: "target address of sshd"},
				},
				Action: server,
			},
			{
				Name: "client",
				Flags: []cli.Flag{
					&cli.StringFlag{Name: "addr", Value: "localhost:4242", Usage: "address of server"},
					&cli.StringFlag{Name: "localaddr", Value: ":0", Usage: "source address of UDP packets"},
				},
				Action: client,
			},
		},
	}
	if err := app.Run(os.Args); err != nil {
		log.Printf("Error: %v", err)
	}
}

func readAndWrite(ctx context.Context, r io.Reader, w io.Writer) <-chan error {
	c := make(chan error)
	go func() {
		defer close(c)

		buff := make([]byte, 8*1024)

		for {
			select {
			case <-ctx.Done():
				c <- er(ctx.Err())
				return
			default:
				nr, err := r.Read(buff)
				if err != nil {
					c <- er(err)
					return
				}
				if nr > 0 {
					_, err := io.Copy(w, bytes.NewReader(buff[:nr]))
					if err != nil {
						c <- er(err)
						return
					}
				}
			}
		}
	}()
	return c
}

func er(e error) error {
	_, f, l, _ := runtime.Caller(1)
	return fmt.Errorf("%s:%d: %w", path.Base(f), l, e)
}
