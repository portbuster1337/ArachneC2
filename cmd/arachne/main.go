package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/portbuster1337/ArachneC2/server/core"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "generate" {
		if err := core.RunGenerate(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		return
	}

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
