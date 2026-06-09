# Operator Design

## Architecture

The "server" is not a traditional server — it's an **operator node** that participates in the
libp2p network as a peer. There is no central infrastructure, no VPS requirement, and no
listening port that needs to be exposed to the internet.

```
┌──────────────────────────────────────────────────────────┐
│                    Operator Node Process                   │
│                                                           │
│  ┌──────────────────────────────────────────────────┐     │
│  │              server/core/operator.go              │     │
│  │  ┌──────────┐  ┌──────────┐  ┌────────────────┐  │     │
│  │  │ RunCLI() │  │ Implant  │  │ handlers       │  │     │
│  │  │ (liner)  │  │ Registry │  │ (ps, ls, exec, │  │     │
│  │  │          │  │          │  │  cd, pwd, d/l, │  │     │
│  │  │          │  │          │  │  upload, shell, │  │     │
│  │  │          │  │          │  │  portfwd, socks)│  │     │
│  │  └────┬─────┘  └────┬─────┘  └────────┬───────┘  │     │
│  │       │              │                 │          │     │
│  │       └──────────────┴─────────────────┘          │     │
│  │                     │                             │     │
│  │              ┌──────┴──────┐                      │     │
│  │              │  transport  │                      │     │
│  │              │  (libp2p)   │                      │     │
│  │              └─────────────┘                      │     │
│  └──────────────────────────────────────────────────┘     │
│                                                           │
│  ┌──────────────────────────────────────────────────┐     │
│  │              server/core/generate.go              │     │
│  │  Cross-compile implants with embedded keys        │     │
│  └──────────────────────────────────────────────────┘     │
└──────────────────────────────────────────────────────────┘
                          │
                    libp2p Network
```

## Directory Structure

```
server/
└── core/
    ├── run.go                 # Entry point: load keys, create operator, start CLI
    ├── operator.go            # Core operator logic (708 lines)
    │   ├── ImplantRecord      # In-memory implant metadata
    │   ├── Operator           # Main struct: keys, node, messenger, implant registry
    │   ├── NewOperator()      # Constructor — creates libp2p node, messenger
    │   ├── Start()            # Connects to network, starts DHT, listens for beacons
    │   ├── handleBeaconStream # Persistent stream reader from implants
    │   ├── handleMessage      # Message dispatch by type
    │   ├── sendCommandToImplant # Sends signed command via direct stream
    │   ├── OpenShell()        # Interactive PTY shell via direct libp2p stream
    │   ├── Portfwd()          # TCP port forwarding through implant
    │   ├── Ls/Cd/Pwd/Ps/Execute/Download/Upload/Kill/Ping
    │   └── disconnectCheckLoop # Detects silent implants via heartbeat timeout
    ├── cli.go                 # Readline-based interactive console (liner)
    │   ├── RunCLI()           # Main loop — parser/dispatch
    │   ├── commandHelp        # Help text for all commands
    │   └── saveHistory/shortenStr
    ├── generate.go            # Implant build system (470 lines)
    │   ├── RunGenerate()      # CLI flag parser
    │   ├── BuildImplant()     # Key generation, source extraction, cross-compilation
    │   ├── prepareBuildDir()  # Extracts embedded source, injects keys
    │   ├── ensureGo()         # Auto-installs Go if missing
    │   ├── ensureGarble()     # Auto-installs garble if missing
    │   └── writeQuietStub()   # Daemonize stub for --quiet mode
    ├── socks_proxy.go         # SOCKS5 proxy through implant (397 lines)
    │   ├── SocksStart/Stop/List
    │   ├── handleSocksConn    # Full SOCKS5 handshake (auth, connect, relay)
    │   ├── LoadSocksCreds/SaveSocksCreds
    │   └── pickImplantPeerID/pickRandomImplant
    └── embedsrc/
        └── embed.go           # Embeds implant_src.tar.gz via Go 1.16 embed
```

## Key Responsibilities

### 1. Implant Discovery & Registration

The operator does NOT actively discover implants. Instead:

1. The operator **advertises itself** in the DHT under the rendezvous key
   `arachne/<operator-peerid>`
2. Implants find the operator via DHT lookup and open a persistent beacon stream
3. The operator also accepts inbound beacon streams (`/bc/1.0.0`) from any peer
4. Each `Z1` (beacon register) is signature-verified against the sender's public key
5. Implants are tracked in an in-memory `map[string]*ImplantRecord` keyed by PeerID

```
Beacon Stream Flow:

Implant opens /bc/1.0.0 stream ──→ Operator accepts in handleBeaconStream
                                    └── Loop:
                                         read length-prefixed protobuf
                                         verify signature
                                         dispatch by envelope.Type
                                         update LastCheckin
```

### 2. Implant Tracking

Each `ImplantRecord` stores:

```go
type ImplantRecord struct {
    Name, Hostname, UUID, Username  string
    UID, GID, OS, Arch              string
    PID                             int32
    PeerID, Version, ActiveC2       string
    Locale                          string
    LastCheckin                     time.Time
    Interval, Jitter                time.Duration
    PublicKey                       crypto.PubKey
    Disconnected                    bool
}
```

A background goroutine (`disconnectCheckLoop`) runs every 30 seconds. Any implant whose
`LastCheckin` exceeds `Interval + Jitter + 30s` is marked `Disconnected = true`.

### 3. Command Dispatch

Commands are sent over **direct libp2p streams** (`/bc/1.0.0/cmd`) — not over pubsub.

```
Operator CLI                   Operator core              Implant
    │                              │                        │
    │  exec("whoami")              │                        │
    │ ─────────────────→           │                        │
    │                              │  Z14{Path:"whoami"}    │
    │                              │  sign envelope         │
    │                              │  open /bc/1.0.0/cmd    │
    │                              │  ───────────────────→ │
    │                              │                        │  exec.Command("whoami")
    │                              │                        │  capture output
    │                              │  Z15{Stdout:"root\n"}  │
    │                              │  ←─────────────────── │
    │  "command sent"              │                        │
    │  ← result printed via        │                        │
    │    handleExecuteResult()     │                        │
```

### 4. Interactive Sessions

Shell, port forwarding, and SOCKS all use **direct libp2p streams** with bidirectional
`io.Copy`. These bypass the envelope/signing layer entirely — once the stream is
established, raw bytes flow in both directions.

| Feature | Protocol ID | How It Works |
|---|---|---|
| Shell | `/x/sh/1.0.0` | Operator sends winsize, implant starts PTY, bidirectional I/O |
| Portfwd | `/x/pf/1.0.0` | Operator sends target address, implant dials TCP, relay |
| SOCKS | `/x/sk/1.0.0` | Operator handles SOCKS5 handshake locally, sends target over stream |

Shell is terminated by typing `exit` or pressing **Ctrl+]** (0x1d byte), which triggers an
escape in the `shellEscaper` reader on the operator side.

### 5. Implant Generation

The operator binary is self-contained — it embeds the entire implant source tree as
`implant_src.tar.gz` via Go's `//go:embed`. At generation time:

1. The embedded tarball is extracted to a temp directory
2. The operator's public key and a fresh implant keypair are written as Go source files
3. Optional build tags are added (antivm, quiet mode)
4. `go build` (or `garble build`) cross-compiles for the target
5. UPX compression and ELF stripping are applied as post-processing

## SOCKS5 Proxy

The operator can act as a SOCKS5 proxy, routing traffic through an implant:

```
SOCKS client (browser) ←→ Operator (SOCKS5 handler) ←libp2p stream→ Implant ←→ Target
```

Features:
- Username/password authentication (SHA-256 hashed, saved to `~/.arachne/socks.json`)
- Random implant selection per request
- Multiple simultaneous proxy instances on different ports

## gRPC RPC Definitions

The protobuf definitions at `protobuf/rpb/rpc.proto` define a service `S` with methods
`M0` through `M13` (opaque names to reduce fingerprinting). These correspond to:

| Method | Input | Output | Purpose |
|---|---|---|---|
| M0 | Empty | Version | Get operator version |
| M1 | Empty | Z27 (implant list) | List registered implants |
| M2 | Z4 | Empty | Kill implant |
| M3 | Z12 | Z13 | List processes |
| M4 | Z14 | Z15 | Execute command |
| M5 | Z16 | Z17 | List directory |
| M6 | Z20 | Z21 | Print working directory |
| M7 | Z22 | Z23 | Download file |
| M8 | Z24 | Z25 | Upload file |
| M9 | Z2 | Z3 | Screenshot |
| M10 | Z5 | Z6 | Open shell |
| M11 | stream Z8 | stream Z8 | Stream tunnel |
| M12 | Z9 | Z9 | SOCKS setup |
| M13 | stream Z10 | stream Z10 | SOCKS data relay |

These are generated but currently unused — the active code uses direct libp2p streams
rather than gRPC for all communication.
