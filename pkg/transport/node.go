package transport

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/libp2p/go-libp2p/p2p/discovery/mdns"
	routingdiscovery "github.com/libp2p/go-libp2p/p2p/discovery/routing"
	"github.com/libp2p/go-libp2p/p2p/host/autorelay"
	"github.com/multiformats/go-multiaddr"
)

const (
	ArachneProtocolID  protocol.ID = "/arachne/1.0.0"
	CommandTopicPrefix  string     = "/arachne/"
	BeaconTopicPrefix   string     = "/arachne/"
	TaskTopicPrefix     string     = "/arachne/"
	CommandsSuffix      string     = "/commands"
	BeaconsSuffix       string     = "/beacons"
	TasksSuffix         string     = "/tasks/"
)

func DefaultBootstrapAddrs() []peer.AddrInfo {
	var out []peer.AddrInfo
	for _, s := range defaultBootstrapPeers {
		m, err := multiaddr.NewMultiaddr(s)
		if err != nil {
			continue
		}
		pi, err := peer.AddrInfoFromP2pAddr(m)
		if err != nil {
			continue
		}
		out = append(out, *pi)
	}
	return out
}

var defaultBootstrapPeers = []string{
	"/dnsaddr/bootstrap.libp2p.io/p2p/QmNnooDu7bfjPFoTZYxMNLWUQJyrVwtbZg5gBMjTezGAJN",
	"/dnsaddr/bootstrap.libp2p.io/p2p/QmQCU2EcMqAqQPR2i9bChDtGNJchTbq5TbXJJ16u19uLTa",
	"/dnsaddr/bootstrap.libp2p.io/p2p/QmbLHAnMoJPWSCR5Zhtx6BHJX9KiKNN6tpvbUcqanj75Nb",
	"/dnsaddr/bootstrap.libp2p.io/p2p/QmcZf59bWwK5XFi76CZX8cbJ4BhTzzA3gU1ZjYZcYW3dwt",
	"/dnsaddr/bootstrap.libp2p.io/p2p/12D3KooWSNjkWJkMhM9D3wPKahRmKwZGQ3KYb3dxf5achHy3G1Uq",
	"/dnsaddr/bootstrap.libp2p.io/p2p/12D3KooWSNjkLwGxn5SkKBzKWHa39shjmAjEZzNzuhNedmCRG5Tc",
	"/dnsaddr/bootstrap.libp2p.io/p2p/12D3KooWSNjkLP1pX2hDp6dGApnsrGeFCKLQcWFLTKBk4iLKofCE",
	"/dnsaddr/bootstrap.libp2p.io/p2p/12D3KooWSNjkM7KMCdmENeFw5KnELi5LKYhJBsWjoJV7BctSgjGQ",
}

type NodeConfig struct {
	ListenAddr     string
	BootstrapPeers []peer.AddrInfo
	EnableRelay    bool
	EnableMDNS     bool
	EnableDHT      bool
	RelayAddrs     []string
	FilterAddrs    func([]multiaddr.Multiaddr) []multiaddr.Multiaddr
	PrivateKey     crypto.PrivKey
}

type Node struct {
	Host    host.Host
	PubSub  *pubsub.PubSub
	DHT     *dht.IpfsDHT
	disc    *routingdiscovery.RoutingDiscovery
	config  NodeConfig
	topics  map[string]*pubsub.Topic
	subs    map[string]*pubsub.Subscription
	mu      sync.RWMutex
	ctx     context.Context
	cancel  context.CancelFunc
}

func NewNode(ctx context.Context, cfg NodeConfig, opts ...libp2p.Option) (*Node, error) {
	ctx, cancel := context.WithCancel(ctx)

	baseOpts := []libp2p.Option{
		libp2p.ListenAddrStrings(cfg.ListenAddr),
		libp2p.NATPortMap(),
	}

	if cfg.PrivateKey != nil {
		baseOpts = append(baseOpts, libp2p.Identity(cfg.PrivateKey))
	}

	if cfg.EnableRelay {
		baseOpts = append(baseOpts, libp2p.EnableRelay())
	}

	var h host.Host
	var err error

	if len(cfg.RelayAddrs) > 0 {
		var relays []peer.AddrInfo
		for _, s := range cfg.RelayAddrs {
			m, err := multiaddr.NewMultiaddr(s)
			if err != nil {
				cancel()
				return nil, fmt.Errorf("parse relay addr %q: %w", s, err)
			}
			pi, err := peer.AddrInfoFromP2pAddr(m)
			if err != nil {
				cancel()
				return nil, fmt.Errorf("parse relay peer info %q: %w", s, err)
			}
			relays = append(relays, *pi)
		}
		baseOpts = append(baseOpts, libp2p.EnableAutoRelayWithStaticRelays(relays))
	} else if cfg.EnableDHT {
		baseOpts = append(baseOpts, libp2p.EnableAutoRelay(
			autorelay.WithPeerSource(func(ctx context.Context, num int) <-chan peer.AddrInfo {
				return findRelayCandidates(ctx, num, h)
			}),
		))
	}

	if cfg.FilterAddrs != nil {
		baseOpts = append(baseOpts, libp2p.AddrsFactory(cfg.FilterAddrs))
	}

	baseOpts = append(baseOpts, opts...)

	h, err = libp2p.New(baseOpts...)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("create libp2p host: %w", err)
	}

	ps, err := pubsub.NewGossipSub(ctx, h)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("create pubsub: %w", err)
	}

	n := &Node{
		Host:   h,
		PubSub: ps,
		config: cfg,
		topics: make(map[string]*pubsub.Topic),
		subs:   make(map[string]*pubsub.Subscription),
		ctx:    ctx,
		cancel: cancel,
	}

	if cfg.EnableDHT {
		d, err := dht.New(ctx, h, dht.Mode(dht.ModeAuto))
		if err != nil {
			cancel()
			return nil, fmt.Errorf("create dht: %w", err)
		}
		n.DHT = d
		n.disc = routingdiscovery.NewRoutingDiscovery(d)
	}

	return n, nil
}

func (n *Node) StartDiscovery() error {
	go func() {
		for _, pi := range n.config.BootstrapPeers {
			connectCtx, cancel := context.WithTimeout(n.ctx, 5*time.Second)
			if err := n.Host.Connect(connectCtx, pi); err != nil {
				log.Printf("[discovery] bootstrap %s: %v", pi.ID.String(), err)
				cancel()
				continue
			}
			cancel()
			log.Printf("[discovery] connected to bootstrap: %s", pi.ID.String())
		}
	}()

	if n.DHT != nil {
		go n.DHT.Bootstrap(n.ctx)
	}

	if n.config.EnableMDNS {
		svc := mdns.NewMdnsService(n.Host, "arachne", &mdnsNotifee{h: n.Host})
		if err := svc.Start(); err != nil {
			return fmt.Errorf("start mdns: %w", err)
		}
	}

	return nil
}

type mdnsNotifee struct {
	h host.Host
}

func (m *mdnsNotifee) HandlePeerFound(pi peer.AddrInfo) {
	if pi.ID == m.h.ID() {
		return
	}
	m.h.Peerstore().AddAddr(pi.ID, pi.Addrs[0], time.Hour)
}

func (n *Node) Advertise(ctx context.Context, ns string) error {
	if n.disc == nil {
		return fmt.Errorf("DHT not enabled")
	}
	_, err := n.disc.Advertise(ctx, ns)
	return err
}

func (n *Node) FindPeers(ctx context.Context, ns string) (<-chan peer.AddrInfo, error) {
	if n.disc == nil {
		return nil, fmt.Errorf("DHT not enabled")
	}
	return n.disc.FindPeers(ctx, ns)
}

func (n *Node) JoinTopic(topic string) (*pubsub.Topic, error) {
	n.mu.Lock()
	defer n.mu.Unlock()

	if t, ok := n.topics[topic]; ok {
		return t, nil
	}

	t, err := n.PubSub.Join(topic)
	if err != nil {
		return nil, fmt.Errorf("join topic %s: %w", topic, err)
	}
	n.topics[topic] = t
	return t, nil
}

func (n *Node) Subscribe(topic string) (*pubsub.Subscription, error) {
	n.mu.Lock()
	defer n.mu.Unlock()

	if s, ok := n.subs[topic]; ok {
		return s, nil
	}

	t, ok := n.topics[topic]
	if !ok {
		var err error
		t, err = n.PubSub.Join(topic)
		if err != nil {
			return nil, fmt.Errorf("join topic %s: %w", topic, err)
		}
		n.topics[topic] = t
	}

	sub, err := t.Subscribe()
	if err != nil {
		return nil, fmt.Errorf("subscribe %s: %w", topic, err)
	}
	n.subs[topic] = sub
	return sub, nil
}

func (n *Node) Publish(ctx context.Context, topic string, data []byte) error {
	t, err := n.JoinTopic(topic)
	if err != nil {
		return err
	}
	return t.Publish(ctx, data)
}

func (n *Node) ConnectToPeer(ctx context.Context, pi peer.AddrInfo) error {
	return n.Host.Connect(ctx, pi)
}

func (n *Node) SetStreamHandler(pid protocol.ID, handler network.StreamHandler) {
	n.Host.SetStreamHandler(pid, handler)
}

func (n *Node) NewStream(ctx context.Context, p peer.ID, pid protocol.ID) (network.Stream, error) {
	return n.Host.NewStream(ctx, p, pid)
}

func (n *Node) Close() error {
	n.cancel()
	if n.DHT != nil {
		n.DHT.Close()
	}
	return n.Host.Close()
}

func (n *Node) Addrs() []multiaddr.Multiaddr {
	return n.Host.Addrs()
}

func (n *Node) ID() peer.ID {
	return n.Host.ID()
}

func (n *Node) AddrsWithID() []multiaddr.Multiaddr {
	var addrs []multiaddr.Multiaddr
	for _, a := range n.Host.Addrs() {
		addrs = append(addrs, a.Encapsulate(multiaddr.StringCast("/p2p/"+n.Host.ID().String())))
	}
	return addrs
}

func findRelayCandidates(ctx context.Context, num int, h host.Host) <-chan peer.AddrInfo {
	ch := make(chan peer.AddrInfo, num)
	go func() {
		defer close(ch)
		if h == nil {
			return
		}
		for _, p := range h.Network().Peers() {
			if len(ch) >= num {
				return
			}
			protocols, err := h.Peerstore().GetProtocols(p)
			if err != nil {
				continue
			}
			for _, proto := range protocols {
				if strings.Contains(string(proto), "circuit/relay") {
					addrs := h.Peerstore().Addrs(p)
					if len(addrs) > 0 {
						select {
						case ch <- peer.AddrInfo{ID: p, Addrs: addrs}:
						case <-ctx.Done():
							return
						}
					}
					break
				}
			}
		}
	}()
	return ch
}
