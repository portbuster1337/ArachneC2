package core

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/portbuster1337/ArachneC2/pkg/cryptography"
)

func Run(relayAddrs []string) error {
	if err := os.MkdirAll(arachneDir(), 0700); err != nil {
		return fmt.Errorf("create arachne directory: %w", err)
	}

	keys, err := cryptography.LoadOrGenerateOperatorKey(keyPath())
	if err != nil {
		return fmt.Errorf("load operator key: %w", err)
	}

	pubPath := pubKeyPath()
	if _, err := os.Stat(pubPath); os.IsNotExist(err) {
		pubBytes, err := crypto.MarshalPublicKey(keys.PublicKey)
		if err == nil {
			if err := os.WriteFile(pubPath, pubBytes, 0644); err == nil {
				log.Printf("[operator] public key exported: %s", pubPath)
			}
		}
	}

	log.Printf("[operator] peer ID: %s", keys.PeerID.String())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	op, err := NewOperator(ctx, keys, relayAddrs)
	if err != nil {
		return fmt.Errorf("create operator: %w", err)
	}
	defer op.Close()

	if err := op.Start(); err != nil {
		return fmt.Errorf("start operator: %w", err)
	}

	op.RunCLI()
	return nil
}

func arachneDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".arachne"
	}
	return filepath.Join(home, ".arachne")
}

func keyPath() string {
	return filepath.Join(arachneDir(), "operator.key")
}

func pubKeyPath() string {
	return filepath.Join(arachneDir(), "operator.pub")
}
