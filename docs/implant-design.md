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
    ├── evasion/         # Anti-analysis, sandbox detection
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

## Core Lifecycle

### Startup
1. Parse embedded config (operator PeerID, bootstrap peers, beacon interval)
2. Generate ephemeral Ed25519 keypair (persisted to disk if possible)
3. Initialize libp2p host with:
   - TCP + WebSocket transports
   - AutoNAT + relay client for NAT traversal
   - GossipSub pubsub
4. Subscribe to `/c/<op>/cx` (commands topic) and its per-implant task topic `/b/<op>/tx/<implant-peerid>`
5. Publish `Z1` (beacon register) to `/b/<op>/bx` (beacons topic)
6. Enter main loop

### Main Loop (Beacon Mode)
```
loop:
  Listen on command topic (with timeout = interval)
  If command received:
    Validate signature against operator key
    Dispatch to handler
    Publish result to beacons topic
  Sleep(interval + random(jitter))
```

### Session Mode
```
on "open-session" command:
  Open direct libp2p stream to operator
  Upgrade to encrypted bidirectional channel
  Handle interactive commands (shell, socks, etc.)
  On disconnect: return to beacon mode
```

## Transport Configuration

The implant tries transports in order:
1. TCP (direct connection to relay/bootstrap peers)
2. WebSocket (for restrictive networks, port 443)
3. WebRTC (via libp2p, for browser-based implants)
4. Circuit relay (via libp2p relay peers)

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
