# Development Roadmap

## Phase 0: Foundation (Weeks 1-2)
- [x] Project structure and documentation
- [x] Go module initialization (`go mod init`)
- [x] libp2p host setup (TCP + WebSocket transports)
- [x] Ed25519 key generation and PeerID derivation
- [x] GossipSub topic subscription and publishing
- [x] Basic Envelope protobuf definition

## Phase 1: Core Protocol (Weeks 3-4)
- [x] Implant registration and beacon model
- [x] Operator node: subscribe to beacons, dispatch commands
- [x] Envelope signing and verification
- [x] Task execution and result delivery
- [x] Reconnect / resilience logic

## Phase 2: Implant Capabilities (Weeks 5-8)
- [x] File system operations (ls, cd, pwd, download, upload)
- [x] Process enumeration (ps)
- [x] Command execution (execute)
- [x] Interactive shell (shell with PTY — direct libp2p stream)
- [x] Screenshot capture (stub)
- [~] Cross-platform support (Linux tested, Windows/macOS pending)
- [x] Beacon interval + jitter configuration

## Phase 3: Interactive Sessions (Weeks 9-10)
- [x] Direct libp2p stream establishment
- [x] Interactive shell with PTY support
- [ ] Reverse SOCKS proxy (implant-side proxy server)
- [ ] Session multiplexing
- [~] Port forwarding → planned via IPFS exfiltration (see Phase 4)

## Phase 4: IPFS Integration (Weeks 11-12)
- [ ] IPFS data exfiltration (CID-based file transfer)
- [ ] IPNS for operator key rotation
- [ ] Public pinning service integration (optional)
- [ ] Filecoin storage deals (future)

## Phase 5: Operator Tooling (Weeks 13-14)
- [x] Implant generation command (build-implant tool)
- [~] Interactive console (readline-based, pre-Cobra)
- [ ] gRPC local API for GUI clients
- [ ] Multi-operator support
- [ ] Full CLI (Cobra) with all commands

## Phase 6: Advanced Features (Weeks 15-16)
- [ ] Evasion techniques (sandbox detection, AMSI bypass)
- [ ] Process injection and migration
- [ ] Windows token manipulation
- [ ] Pivot through compromised implants
- [ ] COFF/BOF loading (inline execution)

## Phase 7: Hardening (Ongoing)
- [ ] End-to-end encryption review
- [ ] Traffic analysis resistance
- [ ] Protocol fuzzing
- [ ] Go routine leak detection
- [ ] Memory safety review of implant

## Tech Stack

| Component | Technology |
|---|---|
| Language | Go (1.22+) |
| P2P Networking | [go-libp2p](https://github.com/libp2p/go-libp2p) |
| PubSub | [go-libp2p-pubsub](https://github.com/libp2p/go-libp2p-pubsub) |
| Serialization | Protocol Buffers (proto3) |
| DHT | [go-libp2p-kad-dht](https://github.com/libp2p/go-libp2p-kad-dht) |
| Relay | [go-libp2p-circuit](https://github.com/libp2p/go-libp2p-circuit) |
| PTY | [creack/pty](https://github.com/creack/pty) |
| IPFS | [go-ipfs-api](https://github.com/ipfs/go-ipfs-api) or embedded |
| CLI | [Cobra](https://github.com/spf13/cobra) |
| TUI | [Bubble Tea](https://github.com/charmbracelet/bubbletea) |
| RPC | [gRPC](https://grpc.io/) |
| DB | SQLite (via [modernc.org/sqlite](https://modernc.org/sqlite)) |
