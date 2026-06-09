# Implant Design

## Overview

The Arachne implant is a lightweight Go binary that connects to the libp2p network and
communicates with its operator entirely through direct p2p streams, with pubsub as a
fallback. It has no hardcoded server IPs or DNS names — only a cryptographic reference
to the operator's public key.

## Directory Structure

```
implant/
├── main.go              # Entry point
└── core/                # All implant runtime logic
    ├── agent.go         # Core lifecycle, beacon loop, command dispatch, handlers
    ├── shell.go         # Shell path selection (/bin/bash, cmd.exe, etc.)
    ├── shell_unix.go    # PTY-based interactive shell (Linux/macOS)
    ├── shell_windows.go # ConPTY-based interactive shell (Windows)
    ├── portfwd.go       # TCP port forwarding tunnel
    ├── socks.go         # SOCKS proxy tunnel (identical to portfwd currently)
    ├── ps.go            # Process listing (Linux /proc, Windows tasklist)
    ├── antivm.go        # VM detection framework (cross-platform, build-tag gated)
    ├── antivm_linux.go  # 50+ Linux-specific VM detection techniques
    ├── antivm_darwin.go # macOS-specific VM detection techniques
    ├── antivm_windows.go# Windows-specific VM detection techniques
    ├── antivm_stub.go   # No-op stub when compiled without `-tags=antivm`
    ├── embedded_pubkey.go        # Generated: operator's public key embedded at build time
    └── embedded_implant_key.go    # Generated: implant's unique keypair embedded at build time
```

## Build Process

1. Operator runs `arachne generate --os linux --arch amd64` (from CLI or standalone)
2. `server/core/generate.go` unpacks the embedded source tree from `server/core/embedsrc/implant_src.tar.gz`
3. Injects the operator's public key and a fresh implant Ed25519 keypair into the source tree
4. Optionally applies build tags (`-tags=antivm`) and quiet-mode stubs
5. Cross-compiles with `CGO_ENABLED=0` for the target OS/arch
6. Applies garble obfuscation (`--obfuscate`), UPX compression (`--upx`), and strip (`--quiet`)
7. The resulting binary is self-contained — no source tree, no external dependencies

Each build generates a unique implant keypair. The implant keeps the same PeerID across restarts.

## Core Lifecycle

### Startup

1. Load operator's public key from embedded `embedded_pubkey.go`
2. Load implant's private key from embedded `embedded_implant_key.go`
3. Derive libp2p PeerID from implant public key
4. Initialize libp2p host with:
   - TCP + WebSocket transports (no UDP/QUIC for sandbox compatibility)
   - AutoNAT + relay client for NAT traversal
   - DHT client for peer discovery
   - GossipSub pubsub
5. If `--peer` flag is provided, connect to operator directly
6. Start DHT discovery loop to find operator via rendezvous namespace
7. Once connected, open persistent beacon stream (`/bc/1.0.0`) to operator
8. Register via signed `Z1` (beacon register with system metadata)
9. Start beacon loop and cover traffic loop

### Main Loop

```
beaconLoop (every 10-15s):
  Send Z1 (beacon register) on persistent /bc/1.0.0 stream
  Sleep(interval + random(jitter))

streamKeepaliveLoop (every 5s):
  Write MsgTypeCover on persistent stream to prevent relay idle timeout

coverTrafficLoop (every 4-7s):
  Publish random noise to the beacon pubsub topic

discoverOperatorLoop (every 15s):
  Find operator via DHT rendezvous
  If beacon stream is nil, reconnect

commandStream handler:
  Read length-prefixed envelope from /bc/1.0.0/cmd stream
  Verify Ed25519 signature against embedded operator pubkey
  Dispatch by message type
  Send result on persistent beacon stream
```

### Session Mode (Shell, Portfwd, SOCKS)

```
on incoming stream:
  Read target/winsize from stream header
  Establish local connection / PTY
  Bidirectional io.Copy between libp2p stream and local resource
  On disconnect or Ctrl+]: cleanup and return
```

## Command Handlers

All command handlers follow the same pattern:

1. Deserialize protobuf request from envelope
2. Execute operation (local filesystem, process execution, etc.)
3. Serialize protobuf result
4. Send result via `sendResult()` — tries persistent stream first, falls back to pubsub

| Handler | Protobuf | Operation |
|---|---|---|
| `handlePs` | Z12 → Z13 | List processes via `/proc` (Linux) or `tasklist` (Windows) |
| `handleLs` | Z16 → Z17 | Read directory entries |
| `handleCd` | Z19 → Z21 | `os.Chdir()` |
| `handlePwd` | Z20 → Z21 | `os.Getwd()` |
| `handleExecute` | Z14 → Z15 | `exec.CommandContext()` with optional output capture |
| `handleDownload` | Z22 → Z23 | `os.ReadFile()` with 100MB limit |
| `handleUpload` | Z24 → Z25 | `os.WriteFile()` with optional overwrite |
| `handleKill` | Z4 → none | Write debug info, `os.Exit(0)` |
| `handlePing` | Ping → Ping | Round-trip keepalive |
| `handleScreenshot` | Z2 → Z3 | Returns "not implemented" stub |

## Platform Support

| Feature | Windows | Linux | macOS |
|---|---|---|---|
| TCP transport | ✓ | ✓ | ✓ |
| WebSocket transport | ✓ | ✓ | ✓ |
| Process list | ✓ (tasklist) | ✓ (/proc) | ✓ (stub) |
| File ops | ✓ | ✓ | ✓ |
| Interactive shell | ✓ (ConPTY) | ✓ (PTY) | ✓ (PTY) |
| Port forwarding | ✓ | ✓ | ✓ |
| VM detection | 50+ checks | 50+ checks | 8 checks |

## Transport Configuration

The implant explicitly disables libp2p's default transports and enables only:

1. **WebSocket** — primary transport, works through most proxies
2. **TCP** — fallback for direct connections

No UDP, no QUIC, no multicast. This maximizes sandbox/container compatibility.

The implant always connects with `network.WithAllowLimitedConn` to work through relay
circuits when direct connections are unavailable.
