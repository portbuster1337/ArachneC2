# Implant Design

## Overview

The Arachne implant is a lightweight Go binary that connects to the libp2p network and
communicates with its operator entirely through decentralized pubsub and direct p2p streams.

## Directory Structure

```
implant/
├── main.go              # Entry point
├── generate.go          # Build-time configuration injection
├── implant.go           # Core implant lifecycle
├── scripts/             # Build scripts
└── arachne/             # Implant runtime
    ├── constants/       # Build-time constants (operator PeerID, etc.)
    ├── cryptography/    # Ed25519 key management, signing, encryption
    ├── encoders/        # Data encoders for staging/evasion
    ├── evasion/         # Anti-analysis, sandbox/VM detection
    ├── handlers/        # Message handler dispatch
    ├── limits/          # Resource limits, watchdog
    ├── priv/            # Privilege escalation helpers
    ├── procdump/        # Process dumping
    ├── ps/              # Process listing
    ├── registry/        # Windows registry
    ├── screen/          # Screenshot capture
    ├── shell/           # Interactive shell / PTY
    ├── transports/      # libp2p transport initialization
    ├── taskrunner/      # Async task execution queue
    ├── pivots/          # Pivot through other implants
    └── util/            # Shared utilities
```

## Build Process

1. Operator runs `arachne generate --os windows --arch amd64`
2. Server injects operator's public key, bootstrap peers, and config
3. Go compiler builds with `-ldflags` for string obfuscation
4. Optional: UPX pack, signature spoofing, or other evasion
5. VM detection compiled in via `--antivm` flag (65+ pure-Go techniques, VMAware-compatible scoring)

## Core Lifecycle

### Startup
1. Parse embedded config (operator PeerID, bootstrap peers, beacon interval)
2. Generate ephemeral Ed25519 keypair (persisted to disk if possible)
3. Initialize libp2p host with:
   - TCP + WebSocket transports
   - AutoNAT + relay client for NAT traversal
   - GossipSub pubsub
4. Start DHT discovery to find operator via rendezvous namespace
5. On DHT connect, open persistent `/bc/1.0.0` stream to operator (relay-aware)
6. Send `Z1` (beacon register) on the persistent stream
7. Start beacon loop (sends `Z1` every 10-15s) and keepalive loop (writes cover traffic every 5s)
8. Start command stream handler (`/bc/1.0.0/cmd`) for incoming commands

### Main Loop (Persistent Beacon Stream)
```
loop:
  Send Z1 (beacon register) on persistent /bc/1.0.0 stream
  Sleep(interval + random(jitter))

parallel goroutines:
  keepalive:  write MsgTypeCover every 5s on persistent stream
  commands:   read /bc/1.0.0/cmd streams, verify, dispatch to handler
              results sent on persistent beacon stream
  DHT:        find operator every 15s, reconnect if beacon stream is nil
```

### Session Mode
```
on "open-session" command:
  Open direct libp2p stream to operator (/x/sh/1.0.0 or /x/pf/1.0.0)
  Upgrade to encrypted bidirectional channel
  Handle interactive commands (shell, portfwd, etc.)
  On disconnect or Ctrl+]: return to main loop
```

## Transport Configuration

The implant uses TCP and WebSocket transports (no UDP/QUIC for sandbox compatibility). Relay circuits use `AllowLimitedConn` to traverse NAT.

1. TCP — direct connection to relay/bootstrap peers
2. WebSocket — for restrictive networks, port 443
3. Circuit relay — via libp2p relay peers (`AllowLimitedConn`)

### Bootstrap Peers
- Compiled-in list of known public libp2p peers
- Falls back to IPFS bootstrap peers if none specified
- Discovers additional peers via DHT on connection

## Platform Support

| Feature | Windows | Linux | macOS |
|---|---|---|---|
| libp2p TCP | ✓ | ✓ | ✓ |
| libp2p WS | ✓ | ✓ | ✓ |
| Process list | ✓ | ✓ | ✓ |
| File ops | ✓ | ✓ | ✓ |
| Shell | ✓ (cmd/pwsh) | ✓ (bash/zsh) | ✓ (zsh/bash) |
| Screenshot | ✓ | ✓ (X11/Wayland) | ✓ |
| Registry | ✓ | - | - |
| Service mgmt | ✓ | ✓ (systemd) | ✓ (launchd) |
| Process injection | ✓ | - | - |
| Token manipulation | ✓ | - | - |
| Named pipe pivot | ✓ | - | - |
