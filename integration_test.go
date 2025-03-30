package main

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net"
	"strings"
	"testing"

	cli "github.com/urfave/cli/v2"
)

const (
	fakeProcName = "quicssh"
	sshdAddr     = "localhost:8822"
	bindAddr     = "localhost:8842"
)

func TestInegratinGoldenFlow(t *testing.T) {
	ctx := context.Background() // TODO use t.Context in go 1.24
	ctx = withLabel(ctx, t.Name())

	// run tcp-echo server (sort of sshd)
	go tcpEchoServer(t)

	// run quicssh server
	go func() {
		noerr(t, app(ctx, []string{fakeProcName, "server", "--bind", bindAddr, "--sshdaddr", sshdAddr}))
	}()

	// run quicssh client
	wr, rd, opt := tweaksStdIO()
	defer rd.Close() // actually, we lost errors
	defer wr.Close() // that will be happened by this closings
	go func() {
		err := app(ctx, []string{fakeProcName, "client", "--addr", bindAddr, "--localaddr", ":0"}, opt)
		if !errors.Is(err, io.EOF) {
			noerr(t, err)
		}
	}()

	// writing data to client
	_, err := wr.Write([]byte("SSH SESSION\n")) // writing one line
	noerr(t, err)

	// reading echo-data from client<-proxyserver<-echotcpserver<-proxyserver
	lineReader := bufio.NewReader(rd)
	line, err := lineReader.ReadString('\n') // waiting for just one echo-line back
	noerr(t, err)

	// doing checks
	const expected = "echo: SSH SESSION\n"
	if line != expected {
		t.Fatalf("expected %q, got %q", expected, line)
	}
}

func TestWrongArgs(t *testing.T) {
	ctx := context.Background()
	t.Run("client_addr", func(t *testing.T) {
		ctx = withLabel(ctx, t.Name())
		err := app(ctx, []string{fakeProcName, "client", "--addr", "x:x"})
		errcont(t, err, ": unknown port")
	})
	t.Run("client_localaddr", func(t *testing.T) {
		ctx = withLabel(ctx, t.Name())
		err := app(ctx, []string{fakeProcName, "client", "--localaddr", "x:x"})
		errcont(t, err, ": unknown port")
	})
	t.Run("server_bind", func(t *testing.T) {
		ctx = withLabel(ctx, t.Name())
		err := app(ctx, []string{fakeProcName, "server", "--bind", "x:x"})
		errcont(t, err, ": unknown port")
	})
	t.Run("server_sshdaddr", func(t *testing.T) {
		ctx = withLabel(ctx, t.Name())
		err := app(ctx, []string{fakeProcName, "server", "--sshdaddr", "x:x"})
		errcont(t, err, ": unknown port")
	})
}

func app(ctx context.Context, args []string, opts ...func(*cli.App)) error {
	app := application()
	for _, o := range opts {
		o(app)
	}
	return app.RunContext(ctx, args)
}

func tweaksStdIO() (io.WriteCloser, io.ReadCloser, func(*cli.App)) {
	r1, w1 := io.Pipe()
	r2, w2 := io.Pipe()
	// returning the opposite end of pipes and option to tweak app
	return w1, r2, func(app *cli.App) {
		app.Reader = r1
		app.Writer = w2
	}
}

func tcpEchoServer(t *testing.T) {
	t.Helper()

	listener, err := net.Listen("tcp", sshdAddr)
	noerr(t, err)

	t.Cleanup(func() { // CAUTION: it is asynchronous
		noerr(t, listener.Close())
	})

	for {
		conn, err := listener.Accept()
		// Because historically they have not exported the error that they return. See issues #4373 and #19252.
		if e, ok := err.(*net.OpError); ok && e.Unwrap().Error() == "use of closed network connection" { //nolint:errorlint
			break
		}
		noerr(t, err)
		// naive approach without any goroutines
		reader := bufio.NewReader(conn)
		for {
			message, err := reader.ReadString('\n')
			if err == io.EOF { //nolint:errorlint
				break
			}
			noerr(t, err)
			// fmt.Println("ECHO SERVER >>>", string(message)) // debug
			_, err = conn.Write([]byte("echo: " + message + "\n"))
			noerr(t, err)
		}
	}
}

func noerr(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("Unexpected error: %s", err.Error())
	}
}

func errcont(t *testing.T, err error, pattern string) {
	t.Helper()
	if err == nil {
		t.Fatal("Unexpected nil error")
	}
	if !strings.Contains(err.Error(), pattern) {
		t.Fatalf("Expected %q, got %s", pattern, err.Error())
	}
}
