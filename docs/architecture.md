# Arachne C2 Architecture

## Overview

Arachne is a decentralized Command & Control (C2) framework inspired by Sliver (BishopFox/sliver),
but built on **libp2p** — the modular peer-to-peer networking stack from Protocol Labs (creators of IPFS).

Instead of hosting centralized C2 servers on VPS/cloud infra (which can be taken down, blocked, or
fingerprinted), Arachne uses the global libp2p DHT + GossipSub for **operator discovery, implant
communications, and command relay** — all without any central server, static IP, or DNS.

## Why libp2p Instead of HTTP/DNS/mTLS/WireGuard (Sliver's Approach)

| Feature | Sliver | Arachne |
|---|---|---|
| Transport | mTLS, HTTP(S), DNS, WireGuard | libp2p (TCP, WebSocket) |
| Server identity | Static IP / domain | PeerID (cryptographic) |
| Discovery | Hardcoded C2 endpoints | DHT + PubSub topic discovery |
| Resilience | Multiple listeners | Any peer can relay |
| Takedown | Block IP/domain | Unbounded: must Sybil the DHT |
| NAT traversal | Manual / WireGuard | AutoNAT + relay + hole-punching |
| Encryption | Per-binary asymmetric keys | libp2p noise/TLS + protobuf envelopes |
| Implant comms | Polling / long-poll / DNS ticks | Direct streams + persistent beacon stream |

## High-Level Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                     libp2p Network (DHT + Relay)             │
│                                                             │
│   ┌──────────┐              ┌──────────┐   ┌──────────┐     │
│   │ Operator │  persistent  │ Implant  │   │ Implant  │     │
│   │ (Client) │←──beacon─────│ (Agent)  │   │ (Agent)  │     │
│   │          │──command────→│          │   │          │     │
│   │          │  direct str  │          │   │          │     │
│   └────┬─────┘              └────┬─────┘   └────┬─────┘     │
│        │                         │              │            │
│        └─────────────────────────┴──────────────┘            │
│                    DHT discovery + relay circuits             │
└─────────────────────────────────────────────────────────────┘
```

## Key Components

### 1. Operator Node (Client)
- Connects to libp2p network with a **PeerID** derived from an operator key
- Handles persistent beacon streams from implants (`/bc/1.0.0`)
- Sends commands to implants over direct libp2p streams (`/bc/1.0.0/cmd`)
- Opens direct libp2p streams for interactive sessions (shell, socks, portfwd)

### 2. Implant Node (Agent)
- Compiled with an **operator's public key** (embedded at build time)
- Connects to IPFS/libp2p bootstrap peers or uses embedded peer list
- Opens persistent beacon stream to operator (`/bc/1.0.0`) with 5s keepalive
- Receives commands on direct streams (`/bc/1.0.0/cmd`) and via pubsub fallback
- Supports session mode (direct stream for shell, portfwd) over relay circuits

### 3. Relay Nodes
- Any libp2p peer can act as a relay (no cost, no registration)
- AutoNAT + relay protocol for NAT traversal
- No special server software — standard libp2p relays

### 4. IPFS Data Layer (Optional)
- Exfiltrated files, screenshots, loot stored as IPFS objects (CID-addressed)
- CIDs sent back via PubSub (small metadata only)
- Data retrieved by operator via IPFS directly (no C2 channel needed for bulk data)

## Communication Model

| Message Type | Transport | Pattern |
|---|---|---|---|
| Beacon / Heartbeat | Persistent direct stream (`/bc/1.0.0`) | Implant → Operator |
| Command dispatch | Direct stream (`/bc/1.0.0/cmd`) | Operator → Implant |
| Task result | Beacon stream or pubsub fallback | Implant → Operator |
| Interactive shell | Direct libp2p stream (`/x/sh/1.0.0`) | Bidirectional (Ctrl+] to exit) |
| File download | Direct stream or IPFS | Stream or IPFS block fetch |
| SOCKS / Portfwd | Direct stream (`/x/pf/1.0.0`) | Proxied through libp2p |
| Pivot | Nested libp2p stream | Implant → Implant → Operator |

## Security Model

- **Implant identity**: Each implant generates an ephemeral keypair on first run, signed by operator's key
- **Encryption**: All libp2p transports are encrypted (Noise XX or TLS 1.3) per spec
- **Message auth**: Envelopes are signed with the sender's private key
- **Operator auth**: Only the operator with the correct private key can publish to `arachne/<op-id>/commands`
- **Forward secrecy**: Ephemeral session keys for direct streams

## Why "Arachne"?

Arachne is the Greek goddess of weaving and spiders — fitting for a framework that weaves a
decentralized web of connections between operator and implants, where any node can be a relay
and the network distributes control across thousands of peers.
