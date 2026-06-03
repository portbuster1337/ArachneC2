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
| Transport | mTLS, HTTP(S), DNS, WireGuard | libp2p (TCP, WebSocket, QUIC) |
| Server identity | Static IP / domain | PeerID (cryptographic) |
| Discovery | Hardcoded C2 endpoints | DHT + PubSub topic discovery |
| Resilience | Multiple listeners | Any peer can relay |
| Takedown | Block IP/domain | Unbounded: must Sybil the DHT |
| NAT traversal | Manual / WireGuard | AutoNAT + relay + hole-punching |
| Encryption | Per-binary asymmetric keys | libp2p noise/TLS + protobuf envelopes |
| Implant comms | Polling / long-poll / DNS ticks | PubSub streaming + direct streams |

## High-Level Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                     libp2p Network (DHT + GossipSub)        │
│                                                             │
│   ┌──────────┐   ┌──────────┐   ┌──────────┐   ┌──────────┐ │
│   │ Operator │   │ Implant  │   │ Implant  │   │ Relay    │ │
│   │ (Client) │   │ (Agent)  │   │ (Agent)  │   │ Node     │ │
│   └────┬─────┘   └────┬─────┘   └────┬─────┘   └────┬─────┘ │
│        │              │              │              │        │
│        └──────────────┴──────────────┴──────────────┘        │
│                         All subscribe to                     │
│                  "/c/<op-id>/cx"                              │
│                  "/b/<op-id>/bx"                              │
└─────────────────────────────────────────────────────────────┘
```

## Key Components

### 1. Operator Node (Client)
- Connects to libp2p network with a **PeerID** derived from an operator key
- Subscribes to `/b/<op-id>/bx` topic for implant check-ins
- Publishes commands on `/c/<op-id>/cx` and per-implant task topics `/b/<op-id>/tx/<implant-id>`
- Opens direct libp2p streams for interactive sessions (shell, socks, portfwd)

### 2. Implant Node (Agent)
- Compiled with an **operator's public key** (embedded at build time)
- Connects to IPFS/libp2p bootstrap peers or uses embedded peer list
- Subscribes to command topic, publishes heartbeat to beacon topic
- Supports beacon mode (async polling via PubSub) and session mode (direct stream)

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
|---|---|---|
| Beacon / Heartbeat | GossipSub topic | Implant -> PubSub -> Operator |
| Command dispatch | GossipSub topic | Operator -> PubSub -> Implant |
| Task result | GossipSub topic | Implant -> PubSub -> Operator |
| Interactive shell | Direct libp2p stream | Bidirectional stream |
| File download | Direct stream or IPFS | Stream or IPFS block fetch |
| SOCKS / Portfwd | Direct stream | Proxied through libp2p |
| Pivot | Nested libp2p stream | Implant -> Implant -> Operator |

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
