package cryptography

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"golang.org/x/crypto/curve25519"
	"golang.org/x/crypto/nacl/box"
)

type KeyPair struct {
	PrivateKey crypto.PrivKey
	PublicKey  crypto.PubKey
}

type BoxKeyPair struct {
	PrivateKey *[32]byte
	PublicKey  *[32]byte
}

type OperatorKey struct {
	KeyPair
	PeerID  peer.ID
	BoxKeys *BoxKeyPair
}

type ImplantKey struct {
	KeyPair
	PeerID          peer.ID
	OperatorPubKey crypto.PubKey
}

func GenerateBoxKey() (*BoxKeyPair, error) {
	pub, priv, err := box.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate box key: %w", err)
	}
	return &BoxKeyPair{PrivateKey: priv, PublicKey: pub}, nil
}

func LoadBoxKey(data []byte) *BoxKeyPair {
	var priv [32]byte
	copy(priv[:], data)
	pub, err := curve25519.X25519(priv[:], curve25519.Basepoint)
	if err != nil {
		return nil
	}
	var pubArr [32]byte
	copy(pubArr[:], pub)
	return &BoxKeyPair{PrivateKey: &priv, PublicKey: &pubArr}
}

func (kp *BoxKeyPair) Marshal() []byte {
	return kp.PrivateKey[:]
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
	boxKeys, err := GenerateBoxKey()
	if err != nil {
		return nil, fmt.Errorf("generate box key: %w", err)
	}
	return &OperatorKey{
		KeyPair: KeyPair{PrivateKey: priv, PublicKey: pub},
		PeerID:  pid,
		BoxKeys: boxKeys,
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
	return priv.Sign(data)
}

func Verify(pub crypto.PubKey, data []byte, sig []byte) (bool, error) {
	return pub.Verify(data, sig)
}

func NewEpochNow() int64 {
	return 0
}

type ReadWriter struct {
	io.Reader
	io.Writer
}

func BoxKeyPath(dir string) string {
	return filepath.Join(dir, "operator.boxkey")
}

func BoxPubKeyPath(dir string) string {
	return filepath.Join(dir, "operator.boxpub")
}

func LoadOrGenerateOperatorKey(path string) (*OperatorKey, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("create key directory: %w", err)
	}

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

		boxPrivData, boxErr := os.ReadFile(BoxKeyPath(dir))
		var boxKeys *BoxKeyPair
		if boxErr == nil && len(boxPrivData) == 32 {
			boxKeys = LoadBoxKey(boxPrivData)
		}
		if boxKeys == nil {
			boxKeys, boxErr = GenerateBoxKey()
			if boxErr == nil {
				os.WriteFile(BoxKeyPath(dir), boxKeys.Marshal(), 0600)
				os.WriteFile(BoxPubKeyPath(dir), boxKeys.BoxPubKeyBytes(), 0644)
			}
		}

		return &OperatorKey{
			KeyPair: KeyPair{PrivateKey: priv, PublicKey: pub},
			PeerID:  pid,
			BoxKeys: boxKeys,
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

	if err := os.WriteFile(path, marshaled, 0600); err != nil {
		return nil, fmt.Errorf("write key to %s: %w", path, err)
	}
	if err := os.WriteFile(BoxKeyPath(dir), key.BoxKeys.Marshal(), 0600); err != nil {
		return nil, fmt.Errorf("write box key: %w", err)
	}
	if err := os.WriteFile(BoxPubKeyPath(dir), key.BoxKeys.BoxPubKeyBytes(), 0644); err != nil {
		return nil, fmt.Errorf("write box pubkey: %w", err)
	}

	return key, nil
}

func (kp *BoxKeyPair) BoxPubKeyBytes() []byte {
	return kp.PublicKey[:]
}

func EncryptMessage(plaintext []byte, boxPub *[32]byte) ([]byte, error) {
	if len(plaintext) == 0 {
		return plaintext, nil
	}
	sealed, err := box.SealAnonymous(nil, plaintext, boxPub, rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("seal anonymous: %w", err)
	}
	return sealed, nil
}

func DecryptMessage(ciphertext []byte, boxKeys *BoxKeyPair) ([]byte, error) {
	if len(ciphertext) == 0 {
		return ciphertext, nil
	}
	opened, ok := box.OpenAnonymous(nil, ciphertext, boxKeys.PublicKey, boxKeys.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("open sealed box failed")
	}
	return opened, nil
}

func (k *OperatorKey) HexPeerID() string {
	return hex.EncodeToString([]byte(k.PeerID))
}

func (k *ImplantKey) HexPeerID() string {
	return hex.EncodeToString([]byte(k.PeerID))
}
