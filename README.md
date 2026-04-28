# TunnelBypass

[![Release](https://img.shields.io/github/v/release/abdelrahman30x/TunnelBypass?style=flat-square&logo=github&color=181717)](https://github.com/abdelrahman30x/TunnelBypass/releases)
[![License](https://img.shields.io/badge/license-MIT-blue.svg?style=flat-square)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.25+-00ADD8?logo=go&style=flat-square)](https://go.dev/dl/)

Spin up tunnel endpoints on **Linux** or **Windows** without wrestling with config files. Pick a transport, answer a few prompts, grab your configs.

No web UI — everything runs in the terminal.

---

## What is it?

TunnelBypass is a CLI tool that sets up server-side tunnel endpoints. It walks you through an interactive wizard, generates client configs and sharing links automatically, and can optionally install itself as a system service.

Built in Go. Single binary. No dependencies.

---

## Features

- 🧙 Interactive wizard — run `tunnelbypass` and follow the prompts
- 🔌 8+ transports out of the box
- ⚡ One-shot mode — `tunnelbypass run <transport> ...` for scripts and automation
- 📦 Auto-generated configs + sharing links — drop into v2rayN, Nekoray, sing-box, WireGuard apps, etc.
- 🪟 Optional Windows service / Linux systemd integration
- 🔒 Sensible defaults with LAN hardening

---

## Supported Transports

**VLESS / Xray**
- `vless+reality` — TCP, XTLS Vision, fronts as real TLS
- `vless+ws+tls` — WebSocket + TLS
- `vless+reality+grpc` — gRPC over REALITY

**SSH & wrappers**
- `ssh` — classic SSH; optional UDPGW for UDP
- `ssh-tls` — TLS front-end (Xray) to SSH backend

**UDP / QUIC**
- `hysteria` — Hysteria 2 over QUIC
- `wireguard` — kernel-style VPN tunnel

**Other**
- `wss` — WebSocket + TLS via wstunnel
- `tls` — TLS wrapper (stunnel-style)

Run `tunnelbypass run -help` for all `--type` values and flags.

---

## Installation

### Linux

```bash
curl -fsSL https://raw.githubusercontent.com/abdelrahman30x/TunnelBypass/main/scripts/install.sh | bash
```

Optional custom prefix:
```bash
INSTALL_PREFIX="$HOME/.local/bin" curl -fsSL ... | bash
```

### Windows

```powershell
irm https://raw.githubusercontent.com/abdelrahman30x/TunnelBypass/main/scripts/install.ps1 | iex
```

> If execution policy blocks it: `Set-ExecutionPolicy -Scope Process -ExecutionPolicy Bypass`, then retry.

### From source

```bash
git clone https://github.com/abdelrahman30x/TunnelBypass.git && cd TunnelBypass
go build -trimpath -ldflags "-s -w" -o tunnelbypass ./cmd
```

Windows: same `go build` command, then `.\tunnelbypass.exe`.

---

## Usage

**Interactive**
```bash
tunnelbypass
```

**One-shot**
```bash
tunnelbypass run portable reality --port 443 --sni example.com --uuid auto
```

**Generate configs only**
```bash
tunnelbypass generate reality --port 443 --sni example.com
```

**Other commands**
```bash
tunnelbypass status      # runtime / service status
tunnelbypass health      # quick health probe
tunnelbypass uninstall   # remove configs, services, and binary
```

---

## Examples

See [`EXAMPLES.md`](EXAMPLES.md) for copy-paste commands and spec files.

---

## LAN Self-Host

When client and server are on the same network, use `--self-host` to resolve a local RFC1918 address instead of a public IP.

```bash
tunnelbypass --self-host              # interactive
tunnelbypass --self-host run reality --port 8443
```

- `--lan-relax` removes the default `geoip:private` block. Only use on isolated home/lab networks.
- Override auto-detection with `TUNNELBYPASS_LAN_IP=192.168.1.10` or `--server <ip>`.

---

## Project Structure

```
cmd/         → entrypoint
internal/    → CLI, engine, networking, config helpers
core/        → provision, installer, transports, service management
tools/       → host catalog utilities
docs/wiki/   → full docs (networking, firewall, Arabic guide, etc.)
```

---

## Tips

- Some transports need **root / admin** (firewall rules, services). Use `--no-elevate` or `run portable` to stay user-level.
- Portable mode: `--portable`, `--data-dir <path>`, or `TUNNELBYPASS_DATA_DIR` to control where configs and logs live.
- Default data paths:
  - Windows: `C:\TunnelBypass\`
  - Linux: `/usr/local/etc/tunnelbypass/`
- Full docs: [`docs/wiki/`](docs/wiki/README.md)
- Arabic beginner guide: [`docs/wiki/Home.md`](docs/wiki/Home.md#arabic-beginners)

---

## Demo

[![TunnelBypass — setup](https://img.youtube.com/vi/praPwuKj9d4/maxresdefault.jpg)](https://www.youtube.com/watch?v=praPwuKj9d4)

---

## License

[MIT](LICENSE)

---

Made with ❤️ in Egypt
