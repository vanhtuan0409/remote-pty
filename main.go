package main

import (
	"bytes"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	"github.com/creack/pty"
	"golang.org/x/crypto/ssh/terminal"
)

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

	buf := &bytes.Buffer{}
	writer := io.MultiWriter(os.Stdout, buf)

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
			go handleConn(conn, buf)
		}
	}()

	go io.Copy(ptmx, os.Stdin)
	io.Copy(writer, ptmx)
}

func handleConn(c net.Conn, input io.Reader) {
	defer c.Close()
	io.Copy(c, input)
}
