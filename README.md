# RouteStack Node Agent

Go agent that manages VPN/proxy infrastructure on Linux nodes under a central controller.

## Architecture

The agent communicates with the RouteStack control plane via mTLS-protected HTTP long-polling. It manages:

- **VPN services**: Xray-core (VLESS Reality/TLS), AmneziaWG
- **Proxy services**: Telemt, MTProto, 3proxy, HAProxy
- **Firewall**: nftables rules
- **TLS certificates**: certbot automation
- **System services**: systemd unit management

### Key design principles

- **No shell execution**: All system operations via Go syscalls and D-Bus
- **Atomic writes**: Config files written to temp file then renamed
- **Context propagation**: Every operation respects `context.Context` cancellation
- **Graceful degradation**: Missing optional binaries logged as warning, not fatal

## Filesystem layout

| Path | Purpose |
|------|---------|
| `/etc/routestack/agent/` | Agent config, TLS certs |
| `/etc/routestack/services/` | Service configs (Xray, HAProxy, etc.) |
| `/etc/routestack/nftables/` | nftables ruleset |
| `/var/lib/routestack/backups/` | Config backups before changes |
| `/var/lib/routestack/components/` | Downloaded component binaries |
| `/var/lib/routestack/state.json` | Agent persistent state |
| `/var/log/routestack/` | Agent and service logs |
| `/run/routestack/` | Runtime state (PID files, sockets) |

## Build

```bash
make build          # local build
make build-linux-amd64  # cross-compile for Linux
make docker-build   # build Docker image
make test           # run tests with race detector
make check          # vet + lint + test
```

## Deployment

### systemd service

```bash
make install
# Binary installed to /usr/local/bin/routestack-agent
```

Create `/etc/systemd/system/routestack-agent.service`.

### Docker (privileged container)

```bash
docker run --privileged --pid=host --net=host \
  -v ./configs:/etc/routestack/agent:ro \
  -v routestack-data:/var/lib/routestack \
  -v /etc/systemd/system:/etc/systemd/system \
  -v /run/systemd:/run/systemd \
  routestack-agent:dev run
```

Or use `docker compose up -d`.

## Configuration

Copy `configs/agent.example.yaml` to `/etc/routestack/agent/config.yaml` and adjust:

- `controller.url` — control plane endpoint
- `controller.ca_cert_path` — path to control plane CA certificate

## Enrollment

Before running, the agent must enroll with the controller:

```bash
routestack-agent enroll --token <enrollment-token> --controller https://panel.example.com
```

This generates an Ed25519 keypair, sends a CSR, and stores the signed certificate for future mTLS connections.

## Commands

```
routestack-agent run                        # Start agent (main loop)
routestack-agent enroll --token <t> ...     # One-shot enrollment
routestack-agent version                    # Print version info
```

## License

Proprietary.
