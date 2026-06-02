package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/portbuster1337/ArachneC2/server/core"
)

func main() {
	var relayAddrs multiFlag
	flag.Var(&relayAddrs, "relay", "relay multiaddress (optional, auto-discovers via DHT by default)")
	flag.Parse()

	if err := core.Run(relayAddrs); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
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
