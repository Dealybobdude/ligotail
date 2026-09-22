# Ligotail

An advanced tunneling/pivoting tool with dual transport: classic TLS/WebSocket and WireGuard-encrypted NAT-traversing channels via [tailcat](https://github.com/tailscale/tailcat).

Built on top of [Ligolo-ng](https://github.com/nicocha30/ligolo-ng) by Nicolas Chatelain.

## What is this?

Ligotail adds **tailcat** as an additional transport layer to Ligolo-ng. Tailcat uses Tailscale's WireGuard data plane to establish encrypted tunnels that automatically traverse NATs, firewalls, and CGNATs — no port forwarding, no public IP required.

You get two agent variants:

| Binary | Transport | Size | Use case |
|--------|-----------|------|----------|
| `ligolo-native-agent` | TLS / WebSocket | ~7 MB | Standard deployment, minimal footprint |
| `ligotail-agent` | TLS / WebSocket + WireGuard (tailcat) | ~20 MB | NAT traversal, no direct connectivity needed |

The proxy always ships with both transports. Both agent types connect to the same proxy — you can mix and match.

## How tailcat transport works

1. Proxy starts a tailcat listener and prints its `tc...` address
2. Agent dials that address using WireGuard (Curve25519 + ChaCha20-Poly1305)
3. Connection bootstraps via DERP relay, then automatically upgrades to direct P2P UDP via STUN hole-punching
4. Once connected, it's a standard `net.Conn` — yamux multiplexing and all Ligolo-ng features (tunnels, listeners, multi-hop) work identically

No Tailscale account, coordination server, or login required. The `tc...` address encodes everything: WireGuard public key, disco key, pre-shared key, and DERP region.

## Quick start

### Proxy

```bash
# Start proxy with both TLS and tailcat listeners
./ligotail-proxy -selfcert -tailcat

# The tailcat address is printed on startup:
#   Tailcat address: tc...
# Use the tailcat_address command in the CLI to retrieve it later
```

### Agent — TLS (classic)

```bash
./ligolo-native-agent -connect proxy.example.com:11601 -ignore-cert
# or with the ligotail agent:
./ligotail-agent -connect proxy.example.com:11601 -ignore-cert
```

### Agent — tailcat (NAT traversal)

```bash
./ligotail-agent -tailcat tc...
```

### Agent — tailcat bind mode

```bash
# Agent listens and prints its own tc... address
./ligotail-agent -tailcat-bind :11601

# On the proxy CLI, connect to the agent:
connect_agent_tailcat --addr tc...
```

## Building

```bash
make all        # Build everything (proxy + all agent variants)
make proxy      # Proxy only (Linux amd64/arm64)
make agents     # All agent variants, all architectures
make clean      # Remove builds/
```

All binaries are statically compiled, stripped, and path-trimmed.

### Agent architectures

| OS | Architectures |
|----|---------------|
| Linux | amd64, 386, arm64, armv7, armv6, mips, mipsle, mips64, mips64le |
| Windows | amd64, 386, arm64 |

MIPS builds use softfloat for embedded device compatibility.

## Build tags

Tailcat support is gated behind the `tailcat` build tag. Building without it produces a slim TLS-only binary:

```bash
# Slim agent (TLS only, ~7 MB)
go build -ldflags="-s -w" ./cmd/agent/

# Full agent (TLS + tailcat, ~20 MB)
go build -tags tailcat -ldflags="-s -w" ./cmd/agent/
```

## Agent flags

### Common (both variants)

| Flag | Description |
|------|-------------|
| `-connect host:port` | Connect to proxy via TLS |
| `-bind ip:port` | Bind mode (TLS) |
| `-ignore-cert` | Skip TLS certificate validation |
| `-accept-fingerprint hex` | Pin TLS certificate by SHA256 fingerprint |
| `-retry` | Auto-retry on initial connection failure |
| `-reconnect` | Auto-reconnect on connection loss (default: true) |
| `-proxy url` | SOCKS5/HTTP proxy for TLS connections |
| `-v` | Verbose output |

### Tailcat-only (ligotail-agent)

| Flag | Description |
|------|-------------|
| `-tailcat tc...` | Connect to proxy via tailcat address |
| `-tailcat-bind :port` | Bind mode via tailcat |
| `-tailcat-port port` | Tailcat port (default: 11601) |

## Proxy flags

### Tailcat-specific

| Flag | Description |
|------|-------------|
| `-tailcat` | Enable tailcat listener alongside TLS |
| `-tailcat-port port` | Tailcat listener port (default: 11601) |

### Proxy CLI commands (tailcat)

| Command | Description |
|---------|-------------|
| `tailcat_address` | Show the proxy's tailcat address |
| `connect_agent_tailcat --addr tc...` | Connect to a bind-mode agent via tailcat |

All standard Ligolo-ng proxy flags and commands remain unchanged. See the [Ligolo-ng documentation](https://docs.ligolo.ng/) for full proxy usage.

## Multi-hop chaining

Both transports support multi-hop pivoting. Example: Proxy <-> Agent-1 (tailcat) <-> Agent-2 (TLS via listener)

1. Connect Agent-1 to the proxy via tailcat
2. On Agent-1, add a listener forwarding to the proxy's TLS port:
   ```
   listener_add --addr 0.0.0.0:11601 --to proxy.example.com:11601 --tcp
   ```
3. Connect Agent-2 through Agent-1's listener:
   ```bash
   ./ligolo-native-agent -connect agent1-ip:11601 -ignore-cert
   ```

## Credits

- **Ligolo-ng** by [Nicolas Chatelain](https://github.com/nicocha30) — the tunneling engine this project is built on
- **Ligolo-ng Web** by [Jeremie Bedjai](https://github.com/jeremiebedjai)
- **tailcat** by [Tailscale](https://github.com/tailscale/tailcat) — WireGuard-based NAT-traversing transport

## License

GPLv3 — see [LICENSE](LICENSE).
