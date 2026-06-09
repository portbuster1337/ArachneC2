# The IP-less / P2P Model

## The Core Idea

Arachne has no server, no domain, and no static IP address.

In traditional C2 (Sliver, Covenant, Cobalt Strike, etc.), the implant is compiled with a
hardcoded IP or domain pointing to a VPS or redirector. That IP/domain is a single point of
failure — block it, seize the server, or sinkhole the DNS, and the entire operation is cut off.

Arachne instead uses **cryptographic addressing** and the **libp2p peer-to-peer network**.
Instead of "connect to 1.2.3.4:443", the implant says "find the peer whose public key matches
this hash, regardless of where it is on the planet." The network handles the discovery and
routing — the operator could be on a laptop behind NAT in a coffee shop, and as long as it's
connected to the libp2p network, implants will find it.

## How Discovery Works (Step by Step)

### 1. Bootstrap — The Only Time You Touch a Known Address

When an implant or operator first starts, it needs to find *someone* on the libp2p network.
It connects to a set of **public bootstrap peers** — the same ones used by IPFS, run by
Protocol Labs:

```
/dnsaddr/bootstrap.libp2p.io/p2p/QmNnooDu7bfjPFoTZYxMNLWUQJyrVwtbZg5gBMjTezGAJN
/dnsaddr/bootstrap.libp2p.io/p2p/QmQCU2EcMqAqQPR2i9bChDtGNJchTbq5TbXJJ16u19uLTa
...
```

These are the *only* hardcoded addresses in the binary. Their sole job is to hand you a
"phone book" (the DHT routing table) and then you never need them again. Because they're
public IPFS infrastructure used by thousands of unrelated peers, Arachne traffic is
indistinguishable from normal IPFS traffic.

The operator binary also ships with IP-based fallback multiaddrs (see
`pkg/transport/bootstrap_ip_fallbacks.go`) so it can bypass DNS entirely if needed.

### 2. DHT — The Distributed Phone Book

Once connected to any peer, the node joins the **Kademlia DHT** (Distributed Hash Table) —
a global, decentralized key-value store spread across every participating node.

The DHT is used for one thing in Arachne: **rendezvous-based discovery**.

```
Operator starts:   Advertises itself under key "arachne/<operator-peerid>"
Implant starts:    Looks up key "arachne/<operator-peerid>" in the DHT
                   Gets back the operator's current multiaddresses
                   Connects directly
```

This is the **only** discovery mechanism. There is no polling, no DNS lookup, no hardcoded
endpoint. The operator can change IPs, move between networks, or go through NAT — and the
DHT always has the current address.

### 3. Relay — Getting Through NAT

If the operator is behind NAT (no public IP, no port forwarding), a direct DHT connection
may fail. libp2p handles this with **circuit relay**:

1. The operator connects to a relay peer (public libp2p node)
2. The relay gives the operator a "virtual address" (`/p2p/<relay-id>/p2p-circuit/p2p/<op-id>`)
3. The operator advertises this relay address in the DHT
4. The implant connects to the relay, which forwards traffic to the operator

Crucially, the relay sees **only encrypted bytes**. It cannot read messages, authenticate
as the operator, or modify traffic. It is a dumb pipe.

Once the implant and operator have a relay connection, libp2p attempts **hole-punching**
to upgrade to a direct connection (bypassing the relay entirely). This happens automatically
and transparently.

```
Phase 1: Implant -> Relay -> Operator  (relayed, slow)
Phase 2: Implant <-> Operator           (direct, fast, after hole-punch)
```

### 4. Persistent Stream — The "Beacon" Without Polling

Once connected, the implant opens a **persistent bidirectional stream** to the operator
(protocol ID `/bc/1.0.0`). This is NOT polling — it's an always-open TCP-like pipe over
the p2p network.

```
Implant sends Z1 (beacon register) on the stream every 10-15 seconds
Operator sends commands back on the same stream (or via separate /bc/1.0.0/cmd stream)
A keepalive goroutine writes cover traffic every 5 seconds to prevent relay timeout
```

If the stream dies (network blip, relay restart), the implant:
1. Re-discovers the operator via DHT
2. Opens a new persistent stream
3. Resumes normal operation

There is no beacon URL, no HTTP callback, no DNS tick. The entire communication is a
single long-lived libp2p stream.

## What the Network Actually Sees

To an observer (ISP, relay operator, IDS), Arachne traffic looks like this:

```
Peer 12D3KooW... connected to Peer 12D3KooX...
Traffic: Noise-encrypted bytes (indistinguishable from any other libp2p traffic)
Topics: /b/<hash>/bx, /c/<hash>/cx (opaque strings, no identifying info)
```

There is no way to distinguish Arachne traffic from:
- An IPFS node syncing content
- A libp2p-based chat application
- Any other application built on the libp2p stack

## Operational Implications

### What You Don't Need

| Traditional C2 | Arachne |
|---|---|
| A VPS or cloud server | Nothing — use the public libp2p network |
| A domain name | No DNS needed |
| A static IP address | Any network, even behind NAT |
| A redirector / CDN | Relays are free and already exist |
| TLS certificates | libp2p Noise handles encryption |
| Firewall rules | No inbound ports needed |
| DNS records | No DNS at all |

### What You Need

1. **Internet access** — the only requirement. The operator needs outbound connectivity to
   reach the libp2p network (any port, any protocol the network allows).

2. **An operator key** — generated on first run and stored at `~/.arachne/operator.key`.
   This is the root of trust. No key = no access.

3. **The operator's public key** — needed to build implants. Export it once, embed it in
   each implant at build time.

### Failure Modes

| Scenario | What Happens |
|---|---|
| Operator goes offline | Implants keep beaconing, detecting disconnect after ~30s. They re-register when operator comes back. |
| DHT is slow | Implants retry every 15s. The operator also discovers implants via inbound beacon streams. |
| Relay goes down | Implant reconnects via DHT to a new relay. Hole-punching happens automatically. |
| Bootstrap peers unreachable | Implants retry. The binary also ships IP-based fallback addrs to bypass DNS. |
| Operator changes network | The DHT is updated within minutes. Implants re-discover via the next discovery cycle. |

## The Role of PubSub (Fallback)

Direct streams are the primary communication channel. **PubSub (GossipSub) is only a fallback**
for when direct streams are unavailable. Topics follow this pattern:

```
/b/<operator-peerid>/bx    — Beacons (implant -> operator, fallback)
/c/<operator-peerid>/cx    — Commands (operator -> implant, fallback)
/t/<operator-peerid>/tx/<id> — Per-implant task topics
```

In normal operation, PubSub is unused. The implant talks to the operator over the
persistent stream, and the operator sends commands over a separate stream.
PubSub only activates if the persistent stream cannot be established or re-established.

## Why This Works at Scale

The libp2p network has **millions of active peers** (IPFS alone). Arachne traffic is a
needle in a haystack. There is no:

- **Traffic fingerprint** — same Noise encryption as every other libp2p connection
- **Timing signature** — cover traffic masks beacon intervals
- **Protocol distinguishability** — same stream multiplexing, same DHT lookups
- **Destination analysis** — no single IP to correlate; the operator's address changes

This is the fundamental difference from any HTTP-based C2: there is no destination
to block because the destination is a cryptographic identity, not a network address.
