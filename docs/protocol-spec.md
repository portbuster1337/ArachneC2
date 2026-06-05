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

## 2. Topic Structure (PubSub Fallback)

Topics are used as a fallback when direct streams are unavailable. Topic IDs use short opaque prefixes to reduce wire fingerprinting.

```
/b/<operator-peerid>/bx    # Beacons — implants -> operator (pubsub fallback)
```

Primary communication uses direct libp2p streams (see section 4).

### Topic Authorization
- `beacons` topic: messages validated against implant's public key
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

| Protocol | ID | Purpose |
|---|---|---|
| Beacon | `/bc/1.0.0` | Persistent beacon stream (implant → operator) |
| Command | `/bc/1.0.0/cmd` | Command delivery (operator → implant) |
| Shell | `/x/sh/1.0.0` | Interactive shell (Ctrl+] to exit) |
| Port forward | `/x/pf/1.0.0` | TCP port forwarding |
| SOCKS | `/x/sk/1.0.0` | SOCKS proxy tunnel |

## 4. Session Types

### 4.1 Beacon Mode (Persistent Stream)
1. Implant opens persistent `/bc/1.0.0` stream to operator (relay-aware, `AllowLimitedConn`)
2. Implant sends signed `Z1` (beacon register) on the stream every 10-15s
3. A separate goroutine writes `MsgTypeCover` envelopes every 5s to prevent relay idle timeout
4. Operator reads messages in a loop, updates `LastCheckin`, dispatches by type
5. Results (`MsgTypeLs`, `MsgTypePs`, etc.) are sent on the same persistent stream
6. If the stream dies, implant reconnects via DHT discovery + `openBeaconStream`

### 4.2 Command Delivery (Direct Stream)
1. Operator opens a `/bc/1.0.0/cmd` stream to implant (relay-aware, `AllowLimitedConn`)
2. Operator signs the envelope and writes it length-prefixed
3. Implant reads, verifies signature against embedded operator pubkey, dispatches
4. Implant processes the command and sends the result on the persistent beacon stream

### 4.3 Interactive Session Mode (Stream)
1. Operator initiates direct libp2p stream to implant
2. Bidirectional encrypted stream for shell/portfwd/socks
3. Uses libp2p stream multiplexing
4. In shell, type `exit` or press **Ctrl+]** to return to the operator prompt

## 5. Message Types

| Type | ID | Direction | Description |
|---|---|---|---|
| COVER | 127 | Implant -> Op | Cover traffic (silently dropped by operator) |
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
| PWD | 12 | Op -> Implant | Print working directory |
| EXECUTE | 13 | Op -> Implant | Run command |
| KILL | 14 | Op -> Implant | Self-terminate |
| DISCONNECT | 255 | Bidirectional | Clean close |

## 6. Data Exfiltration via IPFS

For large data (files, screenshots, logs):
1. Implant pins data to IPFS and gets a CID
2. Implant sends a small envelope containing the CID + encryption key
3. Operator fetches the data from IPFS using the CID
4. Optionally decrypts with the key sent in-band

This keeps the C2 channel low-bandwidth and hard to detect.
