package core

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/portbuster1337/arachne-c2/pkg/cryptography"
)

func Run(relayAddrs []string) error {
	keys, err := cryptography.LoadOrGenerateOperatorKey(keyPath())
	if err != nil {
		return fmt.Errorf("load operator key: %w", err)
	}

	pubPath := pubKeyPath()
	pubBytes, err := crypto.MarshalPublicKey(keys.PublicKey)
	if err == nil {
		if err := os.WriteFile(pubPath, pubBytes, 0644); err == nil {
			log.Printf("[operator] public key exported: %s", pubPath)
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

func keyPath() string {
	exe, err := os.Executable()
	if err == nil {
		return filepath.Join(filepath.Dir(exe), "operator.key")
	}
	return "operator.key"
}

func pubKeyPath() string {
	exe, err := os.Executable()
	if err == nil {
		return filepath.Join(filepath.Dir(exe), "operator.pub")
	}
	return "operator.pub"
}
