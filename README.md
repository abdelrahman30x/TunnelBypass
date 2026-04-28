<p align="center">
  <img src="https://github.com/user-attachments/assets/0a6cd2fa-10ae-43fd-9be1-46be294465bd" alt="TunnelBypass Banner" width="100%">
</p>

<h1>TunnelBypass</h1>

<p align="center">
  <a href="https://github.com/abdelrahman30x/TunnelBypass/releases">
    <img src="https://img.shields.io/github/v/release/abdelrahman30x/TunnelBypass?style=flat-square&logo=github&color=181717" alt="Release">
  </a>
  <a href="LICENSE">
    <img src="https://img.shields.io/badge/license-MIT-blue.svg?style=flat-square" alt="License">
  </a>
  <a href="https://go.dev/dl/">
    <img src="https://img.shields.io/badge/Go-1.25+-00ADD8?logo=go&style=flat-square" alt="Go">
  </a>
</p>

<p>High-performance CLI for Xray, Hysteria & WireGuard. Deploy DPI-resistant tunnel endpoints on <strong>Linux</strong> or <strong>Windows</strong> with one command. Auto-generated configs. No web UI — one binary.</p>

---

<h2>What is it?</h2>

<p>TunnelBypass is a CLI tool that sets up server-side tunnel endpoints. It walks you through an interactive wizard, generates client configs and sharing links automatically, and can optionally install itself as a system service.</p>

<p>Built in Go. Single binary. No dependencies.</p>

---

<h2>✨ Features</h2>

- 🧙 **Interactive wizard** — run `tunnelbypass` and follow the prompts
- 🔌 **8+ transports** out of the box
- ⚡ **One-shot mode** — `tunnelbypass run <transport> ...` for scripts and automation
- 📦 **Auto-generated configs + sharing links** — drop into v2rayN, Nekoray, sing-box, WireGuard apps, etc.
- 🪟 **Optional Windows service / Linux systemd** integration
- 🔒 **Sensible defaults** with LAN hardening

---

<h2>Supported Transports</h2>

<p><strong>VLESS / Xray</strong></p>

- `vless+reality` — TCP, XTLS Vision, fronts as real TLS
- `vless+ws+tls` — WebSocket + TLS
- `vless+reality+grpc` — gRPC over REALITY

<p><strong>SSH & wrappers</strong></p>

- `ssh` — classic SSH; optional UDPGW for UDP
- `ssh-tls` — TLS front-end (Xray) to SSH backend

<p><strong>UDP / QUIC</strong></p>

- `hysteria` — Hysteria 2 over QUIC
- `wireguard` — kernel-style VPN tunnel

<p><strong>Other</strong></p>

- `wss` — WebSocket + TLS via wstunnel
- `tls` — TLS wrapper (stunnel-style)

<p>Run <code>tunnelbypass run -help</code> for all <code>--type</code> values and flags.</p>

---

<h2>Installation</h2>

<h3>Linux</h3>

```bash
curl -fsSL https://raw.githubusercontent.com/abdelrahman30x/TunnelBypass/main/scripts/install.sh | bash
```

Optional custom prefix:
```bash
INSTALL_PREFIX="$HOME/.local/bin" curl -fsSL ... | bash
```

<h3>Windows</h3>

```powershell
irm https://raw.githubusercontent.com/abdelrahman30x/TunnelBypass/main/scripts/install.ps1 | iex
```

> If execution policy blocks it: `Set-ExecutionPolicy -Scope Process -ExecutionPolicy Bypass`, then retry.

<h3>From source</h3>

```bash
git clone https://github.com/abdelrahman30x/TunnelBypass.git && cd TunnelBypass
go build -trimpath -ldflags "-s -w" -o tunnelbypass ./cmd
```

Windows: same `go build` command, then `.\tunnelbypass.exe`.

---

<h2>Usage</h2>

<p><strong>Interactive</strong></p>

```bash
tunnelbypass
```

<p><strong>One-shot</strong></p>

```bash
tunnelbypass run portable reality --port 443 --sni example.com --uuid auto
```

<p><strong>Generate configs only</strong></p>

```bash
tunnelbypass generate reality --port 443 --sni example.com
```

<p><strong>Other commands</strong></p>

```bash
tunnelbypass status      # runtime / service status
tunnelbypass health      # quick health probe
tunnelbypass uninstall   # remove configs, services, and binary
```

---

<h2>Examples</h2>

<p>See <a href="EXAMPLES.md">EXAMPLES.md</a> for copy-paste commands and spec files.</p>

---

<h2>LAN Self-Host</h2>

<p>When client and server are on the same network, use <code>--self-host</code> to resolve a local RFC1918 address instead of a public IP.</p>

```bash
tunnelbypass --self-host              # interactive
tunnelbypass --self-host run reality --port 8443
```

- `--lan-relax` removes the default `geoip:private` block. Only use on isolated home/lab networks.
- Override auto-detection with `TUNNELBYPASS_LAN_IP=192.168.1.10` or `--server <ip>`.

---

<h2>Project Structure</h2>

```
cmd/         → entrypoint
internal/    → CLI, engine, networking, config helpers
core/        → provision, installer, transports, service management
tools/       → host catalog utilities
docs/wiki/   → full docs (networking, firewall, Arabic guide, etc.)
```

---

<h2>Tips</h2>

- Some transports need **root / admin** (firewall rules, services). Use `--no-elevate` or `run portable` to stay user-level.
- Portable mode: `--portable`, `--data-dir <path>`, or `TUNNELBYPASS_DATA_DIR` to control where configs and logs live.
- Default data paths:
  - Windows: `C:\TunnelBypass\`
  - Linux: `/usr/local/etc/tunnelbypass/`
- Full docs: [`docs/wiki/`](docs/wiki/README.md)
- Arabic beginner guide: [`docs/wiki/Home.md`](docs/wiki/Home.md#arabic-beginners)

---

<h2>Demo</h2>

<p align="center">
  <a href="https://www.youtube.com/watch?v=praPwuKj9d4">
    <img src="https://img.youtube.com/vi/praPwuKj9d4/maxresdefault.jpg" alt="TunnelBypass — setup" width="70%">
  </a>
</p>

---

<h2>License</h2>

<p><a href="LICENSE">MIT</a></p>

---

<p align="center">Made with ❤️ in Egypt</p>
