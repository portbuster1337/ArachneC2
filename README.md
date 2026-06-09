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

- Self-contained single binary — no source tree needed to generate implants
- Beacon-mode implants that maintain presence via PubSub topics
- DHT-based peer discovery — no hardcoded server IPs
- Encrypted and signed messages (Ed25519 + NaCl box)
- Interactive operator console (list, select, exec, ls, ps, cd, pwd, download, upload)
- Interactive shell over direct libp2p stream (PTY on Linux/macOS, hidden ConPTY on Windows)
- Port forwarding through implant via direct libp2p stream
- Cross-platform implants (Linux, macOS, Windows)
- Protocol Buffers message format with per-message signature verification
- Opaque protocol identifiers (short proto package names, Z-series message types, short wire IDs)
- Per-implant command topics — commands reach only the intended implant
- Built-in hole punching and NAT traversal
- Automatic Go installation if missing (generates implants anywhere)
- Garble-based obfuscation (`--obfuscate` — strips names, literals, paths)
- Cover traffic to mask beacon timing signatures
- Persistent implant identity (embedded keypair per build)
- Quiet mode (`--quiet` — daemonize on Linux/macOS, hide console on Windows)
- Automatic implant disconnect detection and alerting
- Stream keepalive prevents relay circuit idle timeout
- WebSocket + TCP transport (UDP/multicast-free for sandbox compatibility)
- VM detection (`--antivm` — 65+ detection techniques with VMAware-compatible scoring, no CGO required)

## Project Structure

```
arachne-c2/
├── build.sh                # Build script (auto-installs Go if missing)
├── bin/                    # Compiled binaries
├── cmd/
│   └── arachne/            # Single entry point (serve + generate)
├── docs/                   # Design documentation
├── implant/                # Implant agent code
│   └── core/               # Agent runtime, command handlers, shell, portfwd
├── pkg/
│   ├── config/             # Shared config types
│   ├── cryptography/       # Ed25519 + NaCl key management
│   └── transport/          # libp2p node, messenger, PubSub helpers
├── protobuf/               # Protocol Buffers definitions
│   ├── apb/                # C2 protocol messages (opaque type names: Z1, Z2, ...)
│   ├── cpb/                # Common types (Process, Response, Request)
│   └── rpb/                # RPC service definitions (service S, methods M0-M13)
└── server/                 # Operator node (the "server")
    └── core/               # Operator logic, implant tracking, CLI, generate
```

## Build

The operator binary is self-contained — embed the implant source at build time, then it builds implants anywhere:

```bash
./build.sh       # auto-installs Go if missing, embeds source, cross-compiles for all platforms
```

Binaries are written to `bin/` as `arachne-{os}-{arch}` (or `*.exe` for Windows). The built binary can be copied to any machine with Go installed (or no Go — it auto-installs). No source tree needed.

## Quick Start

### 1. Run the operator

```bash
./bin/arachne
```

On first run, generates a keypair at `~/.arachne/operator.key` and exports the public
key to `~/.arachne/operator.pub`.

### 2. Build an implant

From the operator console (`generate`) or standalone:

```bash
./bin/arachne generate --os linux --arch amd64 --output ./myimplant --upx
```

Flags: `--os` (linux, darwin, windows), `--arch` (amd64, arm64), `--output`, `--pubkey`, `--upx` (default true), `--obfuscate`, `--quiet`, `--antivm`.

Use `--obfuscate` to strip function names, package paths, and literal strings via [garble](https://github.com/burrowers/garble) (auto-installed if missing). Combine with `--upx` for maximum hardening.

Use `--antivm` to compile in VM detection. The implant runs 65+ detection techniques (CPUID signatures, MAC prefixes, DMI/SMBIOS, PCI vendor IDs, process enumeration, registry keys, container detection) with a [VMAware](https://github.com/kernelwernel/VMAware)-compatible accumulated scoring system. Exits cleanly if the score exceeds 50%. Pure Go — no CGO, no cross-compilers needed.

Each build generates a unique embedded keypair — the implant keeps the same PeerID across restarts.

### 3. Deploy and run the implant

Copy `./myimplant` to the target machine and run:

```bash
./myimplant
```

The implant will discover the operator via DHT, register itself, and begin beaconing
over a persistent relay stream with automatic keepalive.

### 4. Use the operator console

From the operator prompt:

```
arachne> list
  0: user@hostname [linux/amd64] last=5s peer=12D3Koo...

arachne> select 0
arachne (user@hostname) > exec whoami
```

Available commands: `list`, `select <n>`, `exec <cmd>`, `ls <path>`, `cd <path>`,
`pwd`, `ps`, `shell`, `portfwd <port> <host:p>`, `download <path>`, `upload <path>`,
`generate [flags]`, `regenerate`, `help [command]`, `exit`.

In the interactive shell, type `exit` or press **Ctrl+]** to return to the arachne prompt.

Use `help <command>` or `<command> --help` for per-command details.

## Available Commands

| Command | Description |
|---|---|
| `list` | Show registered implants |
| `select <idx>` | Select implant by index |
| `ps` | List processes on selected implant |
| `ls <path>` | List directory |
| `cd <path>` | Change directory |
| `pwd` | Print working directory |
| `shell` | Interactive shell (direct libp2p stream, Ctrl+] to exit) |
| `portfwd <port> <host:p>` | Forward local port through implant |
| `exec <cmd> [args]` | Execute command (with output) |
| `download <path>` | Download file from implant |
| `upload <src> <dst>` | Upload file to implant |
| `generate [flags]` | Build an implant for any OS/arch (auto-installs Go, garble). Flags: `--os`, `--arch`, `--output`, `--pubkey`, `--upx`, `--obfuscate`, `--quiet`, `--antivm` |
| `regenerate` | Regenerate operator keypair (old implants orphaned) |
| `help [command]` | This help, or details for a specific command |
| `exit` | Quit |

## Security

- All messages are signed with Ed25519 keys
- Implants are built with the operator's public key embedded — they will only accept
  commands from that operator
- Each implant has a unique embedded keypair (persistent identity across restarts)
- Cover traffic masks beacon timing against network observers
- Peer identity is verified on every message via envelope signatures
- Relay nodes see only encrypted bytes — cannot read or modify traffic

## Disclaimer

This software is provided for **educational and authorized security testing purposes only**.
You must only use Arachne C2 on systems you own or have explicit written permission to test.
Unauthorized access to computer systems is illegal. The authors assume no liability and are
not responsible for any misuse or damage caused by this program.

## License

GPLv3
