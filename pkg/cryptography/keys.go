package cryptography

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
)

type KeyPair struct {
	PrivateKey crypto.PrivKey
	PublicKey  crypto.PubKey
}

type OperatorKey struct {
	KeyPair
	PeerID peer.ID
}

type ImplantKey struct {
	KeyPair
	PeerID   peer.ID
	OperatorPubKey crypto.PubKey
}

func GenerateOperatorKey() (*OperatorKey, error) {
	priv, pub, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate operator key: %w", err)
	}
	pid, err := peer.IDFromPublicKey(pub)
	if err != nil {
		return nil, fmt.Errorf("peer id from pubkey: %w", err)
	}
	return &OperatorKey{
		KeyPair: KeyPair{PrivateKey: priv, PublicKey: pub},
		PeerID:  pid,
	}, nil
}

func GenerateImplantKey(operatorPub crypto.PubKey) (*ImplantKey, error) {
	priv, pub, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate implant key: %w", err)
	}
	pid, err := peer.IDFromPublicKey(pub)
	if err != nil {
		return nil, fmt.Errorf("peer id from pubkey: %w", err)
	}
	return &ImplantKey{
		KeyPair:        KeyPair{PrivateKey: priv, PublicKey: pub},
		PeerID:         pid,
		OperatorPubKey: operatorPub,
	}, nil
}

func LoadPrivateKey(data []byte) (crypto.PrivKey, error) {
	return crypto.UnmarshalPrivateKey(data)
}

func MarshalPrivateKey(priv crypto.PrivKey) ([]byte, error) {
	return crypto.MarshalPrivateKey(priv)
}

func PubKeyFromBytes(data []byte) (crypto.PubKey, error) {
	return crypto.UnmarshalPublicKey(data)
}

func PeerIDFromPubKey(pub crypto.PubKey) (peer.ID, error) {
	return peer.IDFromPublicKey(pub)
}

func PeerIDFromBytes(data []byte) (peer.ID, error) {
	pub, err := PubKeyFromBytes(data)
	if err != nil {
		return "", err
	}
	return PeerIDFromPubKey(pub)
}

func Sign(priv crypto.PrivKey, data []byte) ([]byte, error) {
	ed25519Key, ok := priv.(*crypto.Ed25519PrivateKey)
	if !ok {
		raw, err := priv.Raw()
		if err != nil {
			return nil, fmt.Errorf("get raw private key: %w", err)
		}
		return ed25519.Sign(ed25519.NewKeyFromSeed(raw), data), nil
	}
	raw, err := ed25519Key.Raw()
	if err != nil {
		return nil, fmt.Errorf("get raw ed25519 key: %w", err)
	}
	return ed25519.Sign(ed25519.NewKeyFromSeed(raw), data), nil
}

func Verify(pub crypto.PubKey, data []byte, sig []byte) (bool, error) {
	raw, err := pub.Raw()
	if err != nil {
		return false, fmt.Errorf("get raw public key: %w", err)
	}
	return ed25519.Verify(raw, data, sig), nil
}

func NewEpochNow() int64 {
	return 0
}

type ReadWriter struct {
	io.Reader
	io.Writer
}

func LoadOrGenerateOperatorKey(path string) (*OperatorKey, error) {
	data, err := os.ReadFile(path)
	if err == nil {
		priv, err := LoadPrivateKey(data)
		if err != nil {
			return nil, fmt.Errorf("load private key from %s: %w", path, err)
		}
		pub := priv.GetPublic()
		pid, err := peer.IDFromPublicKey(pub)
		if err != nil {
			return nil, fmt.Errorf("peer id from loaded key: %w", err)
		}
		return &OperatorKey{
			KeyPair: KeyPair{PrivateKey: priv, PublicKey: pub},
			PeerID:  pid,
		}, nil
	}

	if !os.IsNotExist(err) {
		return nil, fmt.Errorf("read key file %s: %w", path, err)
	}

	key, err := GenerateOperatorKey()
	if err != nil {
		return nil, err
	}

	marshaled, err := MarshalPrivateKey(key.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("marshal key: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, fmt.Errorf("create key directory: %w", err)
	}

	if err := os.WriteFile(path, marshaled, 0600); err != nil {
		return nil, fmt.Errorf("write key to %s: %w", path, err)
	}

	return key, nil
}

func (k *OperatorKey) HexPeerID() string {
	return hex.EncodeToString([]byte(k.PeerID))
}

func (k *ImplantKey) HexPeerID() string {
	return hex.EncodeToString([]byte(k.PeerID))
}
