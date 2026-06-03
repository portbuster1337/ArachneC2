# Server & Operator Design

## Architecture

The "server" is not a traditional server — it's an **operator node** that participates in the
libp2p network as a peer. No central infrastructure is required beyond what the libp2p network
provides (DHT, relays, bootstrap peers).

```
┌─────────────────────────────────────────────────────────────┐
│                   Operator Node Process                      │
│                                                             │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌────────────┐  │
│  │ CLI/GUI  │  │ Console  │  │ RPC      │  │ Background │  │
│  │ (Cobra)  │  │ (TUI)    │  │ (gRPC)   │  │ Services   │  │
│  └────┬─────┘  └────┬─────┘  └────┬─────┘  └─────┬──────┘  │
│       │              │              │              │         │
│       └──────────────┴──────────────┴──────────────┘         │
│                          │                                   │
│                    ┌─────┴──────┐                            │
│                    │   Core     │                            │
│                    │  (Events,  │                            │
│                    │   State,   │                            │
│                    │   Dispatch)│                            │
│                    └─────┬──────┘                            │
│                          │                                   │
│                    ┌─────┴──────┐                            │
│                    │  Transport │                            │
│                    │  (libp2p)  │                            │
│                    └────────────┘                            │
└─────────────────────────────────────────────────────────────┘
                          │
                    libp2p Network
```

## Directory Structure

```
server/
├── main.go              # Server node entry point
├── README.md
├── core/                # Core orchestration
│   ├── state.go         # Implant registry, session state
│   ├── events.go        # Event bus for internal dispatch
│   └── dispatch.go      # Task routing to implants
├── transport/           # libp2p setup
│   ├── host.go          # libp2p host initialization
│   ├── pubsub.go        # GossipSub topic management
│   └── streams.go       # Direct stream handling
├── handlers/            # Message handlers
│   ├── beacon.go        # Beacon registration & check-in processing
│   ├── commands.go      # Command dispatch
│   └── interact.go      # Interactive session handlers
├── generate/            # Implant generation
│   ├── generate.go      # Build orchestration
│   └── templates/       # Go source templates
├── loot/                # Exfiltrated data management
│   ├── store.go         # Local storage
│   └── ipfs.go          # IPFS integration for retrieval
├── cryptography/        # Key management
│   ├── keys.go          # Operator key generation
│   └── signing.go       # Message signing & verification
├── console/             # Interactive console
│   ├── tui.go           # Terminal UI
│   └── commands.go      # Operator command handlers
├── cli/                 # CLI entry points
│   └── root.go          # Cobra root command
├── db/                  # Local database
│   ├── sqlite.go        # SQLite for implant metadata
│   └── migrations/      # Schema migrations
└── configs/             # Configuration management
    └── config.go        # Operator config loading
```

## Key Responsibilities

### 1. Implant Discovery & Management
- Listen on `/b/<op>/bx` (beacons topic)
- Accept `Z1` (beacon register) messages and validate signatures
- Maintain in-memory and SQLite-backed implant registry
- Track implant state: online/offline/last-checkin

### 2. Command Dispatch
- Subscribe to beacons to detect active implants
- Accept operator commands from CLI
- Publish tasks on `/b/<op>/tx/<implant-id>` (per-implant task topic)
- Await results on beacon topic or via stream

### 3. Interactive Sessions
- Open direct libp2p stream to implant (via DHT lookup or relay)
- Bidirectional shell, port forwarding, SOCKS proxy
- Stream multiplexing for concurrent channels

### 4. Implant Generation
- Compile implant binaries with operator's public key embedded
- Cross-compilation for Windows/Linux/macOS
- Support for staged payloads (small stager downloads full implant)
- Signature spoofing and evasion options

### 5. Loot / Data Retrieval
- Receive CIDs from implants via beacon messages
- Fetch data from IPFS (local IPFS node or public gateway)
- Store locally with metadata

## Multi-Operator Support

Multiple operators can control the same implant fleet:
- Each operator has their own keypair
- Implants can be built with multiple operator public keys
- Each operator subscribes to their own `/b/<op>/bx` (beacons topic)
- Commands are signed and implants verify before execution

## gRPC API (Local)

The operator node exposes a local gRPC API (bound to localhost) for:
- GUI clients to connect
- External tooling integration
- Remote operator access via authenticated tunnel

```protobuf
service S {
  rpc M0(Empty) returns (Implants);
  rpc M1(ImplantID) returns (Implant);
  rpc M2(CommandRequest) returns (CommandResponse);
  rpc M3(SessionRequest) returns (stream SessionData);
  rpc M4(GenerateRequest) returns (GenerateResponse);
}
```
