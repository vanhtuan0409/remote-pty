package main

import (
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"sync"
	"syscall"

	"github.com/creack/pty"
	"golang.org/x/crypto/ssh/terminal"
)

type broadcaster struct {
	sync.Mutex
	outs map[string]io.Writer
}

func newBroadcaster() *broadcaster {
	return &broadcaster{
		outs: make(map[string]io.Writer),
	}
}

func (b *broadcaster) subscribe(ident string, out io.Writer) {
	b.Lock()
	defer b.Unlock()
	b.outs[ident] = out
}

func (b *broadcaster) unsubscribe(ident string) {
	delete(b.outs, ident)
}

func (b *broadcaster) Write(p []byte) (n int, err error) {
	b.Lock()
	defer b.Unlock()
	for ident, w := range b.outs {
		_, err := w.Write(p)
		if err != nil {
			b.unsubscribe(ident)
		}
	}
	return len(p), nil
}

func main() {
	command := exec.Command("zsh")
	ptmx, err := pty.Start(command)
	if err != nil {
		panic(err)
	}
	defer ptmx.Close()

	// handle resize terminal events
	resizeCh := make(chan os.Signal, 1)
	signal.Notify(resizeCh, syscall.SIGWINCH)
	go func() {
		for range resizeCh {
			if err := pty.InheritSize(os.Stdin, ptmx); err != nil {
				log.Printf("Failed to resize PTY. ERR: %v", err)
			}
		}
	}()
	resizeCh <- syscall.SIGWINCH //initial resize

	// handle keyboard events
	oldState, err := terminal.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		panic(err)
	}
	defer terminal.Restore(int(os.Stdin.Fd()), oldState)

	ptyBroadcaster := newBroadcaster()
	ptyBroadcaster.subscribe("stdout", os.Stdout)

	l, err := net.Listen("tcp", ":8080")
	if err != nil {
		panic(err)
	}

	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				log.Printf("Cannot accpet connection. ERR: %v", err)
				continue
			}
			go func(c net.Conn) {
				connReader, connWriter := io.Pipe()
				defer connReader.Close()
				defer connWriter.Close()
				ptyBroadcaster.subscribe(c.RemoteAddr().String(), connWriter)
				handleConn(c, connReader)
			}(conn)
		}
	}()

	go io.Copy(ptmx, os.Stdin)
	io.Copy(ptyBroadcaster, ptmx)
}

func handleConn(c net.Conn, input io.Reader) {
	defer c.Close()
	io.Copy(c, input)
}
