# IPFS & Decentralized Infrastructure Integration

## libp2p (Used)

| Feature | Use in Arachne |
|---|---|
| **Peer Identity** | Cryptographic identity (Ed25519) for operator, implants, relays |
| **GossipSub** | PubSub topics for command/beacon messaging |
| **DHT** | Peer discovery, relay discovery, content routing |
| **AutoNAT** | NAT status detection |
| **Circuit Relay** | Relay connections when direct dial fails |
| **Hole Punching** | Direct p2p connections through NAT |
| **Stream Multiplexing** | Multiple concurrent channels per connection |
| **Noise/TLS Security** | Encrypted, authenticated transports |

## IPFS (Not Yet Implemented)

The `Z11` protobuf message type in `apb/arachne.proto` defines a data reference
format (CID, encryption key, filename, size) for future IPFS-based exfiltration,
but no code currently reads or responds to it. The screenshot handler in
`implant/core/agent.go` returns a "not implemented on this platform" stub.
