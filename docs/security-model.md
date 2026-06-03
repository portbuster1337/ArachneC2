# Security Model — Sole Ownership & Permissioned Access

## Core Principle

**You are the sole owner of your data. No one else — not relay operators, not IPFS nodes, not
ISP observers — can read your C2 traffic, identify your implants, or access exfiltrated data.**

Unlike centralized C2 frameworks where a server operator (or hosting provider, or law enforcement)
could seize the server and read everything, Arachne's decentralized model ensures that
cryptographic ownership and access control are built into every layer.

## End-to-End Ownership

### 1. Operator Key = Ownership

The operator's Ed25519 private key is the root of trust. It is:
- **Never shared** — not with relays, not with bootstrap peers, not with IPFS nodes
- **Never stored on any network** — only on the operator's local machine
- **The sole credential** that can authorize implants and decrypt data

```
Operator Private Key (NEVER leaves operator's machine)
    ├── Derives Operator PeerID (public identity)
    ├── Signs implant binaries (proves ownership)
    ├── Signs all commands (authenticity)
    └── Encrypts exfil data keys (confidentiality)
```

### 2. What a Relay/Bootstrap Peer Sees

A libp2p relay forwards encrypted traffic. It sees:
- `Source PeerID -> Destination PeerID` (who is talking to whom)
- Encrypted bytes (cannot read contents)
- PubSub topic names (e.g., `/c/<peerid>/cx`) — topic IDs are hashes of the operator's public key

Relays **cannot**:
- Decrypt message contents
- Authenticate as the operator
- Send commands to implants
- Distinguish C2 traffic from any other libp2p traffic
- Identify the data as C2 traffic

### 3. What an IPFS Node Sees

When using IPFS for data exfiltration, IPFS nodes see:
- Encrypted blocks with content hashes (CIDs)
- No knowledge of what the data is or who it belongs to
- No ability to decrypt without the one-time key sent in-band over the C2 channel

IPFS nodes **cannot**:
- Decrypt exfiltrated files (encrypted before uploading to IPFS)
- Correlate blocks to a specific operator
- Distinguish Arachne data from any other IPFS content

```
Exfiltration Flow:

Implant:
  1. Encrypt data with one-time AES-256-GCM key
  2. Upload encrypted blob to IPFS -> CID: QmEncryptedBlob
  3. Send over C2 channel (encrypted via libp2p Noise):
     { type: EXFIL, cid: QmEncryptedBlob, key: <one-time-key> }

Network observer sees:    IPFS block transfer (indistinguishable)
Operator sees:            Decrypted file contents
```

## Layered Encryption

| Layer | Protocol | Protects |
|---|---|---|
| Transport | libp2p Noise XX / TLS 1.3 | Eavesdropping, MITM on wire |
| PubSub | Envelope signing (Ed25519) | Impersonation, replay |
| Message | Per-message signing | Integrity, authenticity |
| Exfil | One-time AES-256-GCM + CID | Data at rest on IPFS |
| Command | Signed envelopes | Only operator can send commands |

## Threat Model & Guarantees

### Operator is compromised
- **Impact**: Total loss — attacker controls everything
- **Mitigation**: Multi-operator support; each operator has their own key
- **Hardware key support** (future): Store operator key on YubiKey / TPM

### libp2p relay is malicious
- **Impact**: Can see PeerIDs communicating, drop relay traffic
- **Mitigation**: Relays only see encrypted bytes; hole-punching avoids relays after connection
- **Cannot**: Read messages, impersonate operator, modify traffic

### IPFS node is malicious
- **Impact**: Can refuse to serve blocks (DoS)
- **Mitigation**: Fetch via multiple gateways; pin on multiple services
- **Cannot**: Decrypt blocks (AES-256-GCM), know what data is

### DHT / Sybil attack
- **Impact**: Attacker could intercept peer discovery
- **Mitigation**: Implants store direct backup peer list; operator PeerID is signed
- **Cannot**: Forge operator identity, decrypt messages

### Social / Traffic analysis
- **Impact**: Observer sees that "PeerID X talks to PeerID Y"
- **Mitigation**:
  - Implants can use random PeerIDs per session
  - Cover traffic (random noise published to topics)
  - Traffic indistinguishable from normal libp2p/IPFS usage

## Data Sovereignty Checklist

| Concern | How Arachne Addresses It |
|---|---|
| Who owns the data? | Only the operator — data encrypted before touching the network |
| Who can read commands? | Only implants with the operator's public key embedded |
| Who can send commands? | Only the operator with the private key |
| Who can read exfil? | Only the operator — one-time key sent over encrypted C2 channel |
| Who knows implant locations? | Only the operator (discovery via signed DHT records) |
| Can a relay block me? | Yes, but any libp2p relay works — use multiple |
| Can IPFS lose my data? | Yes, if unpinned — use pinning services or Filecoin |
| Can traffic be identified as C2? | No — indistinguishable from regular p2p traffic |

## Operational Security Recommendations

1. **Generate operator key on an air-gapped machine** and transfer only the public key
2. **Use unique implant keypairs** — never reuse across engagements
3. **Rotate operator key** periodically (use IPNS for key rotation)
4. **Pin exfiltrated data** to at least 3 independent pinning services
5. **Enable cover traffic** to mask beacon timing signatures
6. **Use ephemeral PeerIDs** for implants (regenerate on each run)
7. **Self-host a libp2p relay** for resilience against public relay rate limits
8. **Prefer hole-punching** over relayed connections for sensitive sessions
