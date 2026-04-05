# TunnelBypass

[![GitHub](https://img.shields.io/badge/GitHub-TunnelBypass-181717?logo=github)](https://github.com/abdelrahman30x/TunnelBypass)
[![License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.25+-00ADD8?logo=go)](https://go.dev/dl/)

Command-line tool to set up tunnel endpoints on a **Linux** or **Windows** server: guided flow, generated configs, and optional services.

### Supported transports

| Transport | What it is |
| :-- | :-- |
| **VLESS + REALITY** | TCP, XTLS Vision — looks like TLS to a real site |
| **VLESS + WebSocket + TLS** | VLESS over WebSocket + TLS |
| **VLESS + REALITY + gRPC** | gRPC (HTTP/2) inside REALITY |
| **SSH-TLS** | TLS front-end (Xray) to SSH backend |
| **Hysteria** | QUIC / UDP (Hy2) |
| **WireGuard** | Kernel-style VPN tunnel |
| **SSH** | Classic SSH; optional **UDPGW** for UDP |
| **WSS** | WebSocket + TLS via **wstunnel** |
| **TLS** | TLS wrapper (**stunnel**-style) |

Run `tunnelbypass run -help` for `--type` names and flags.

## Quickstart

### Linux

**Install:**

```bash
curl -fsSL https://raw.githubusercontent.com/abdelrahman30x/TunnelBypass/main/scripts/install.sh | bash
```

Optional: `INSTALL_PREFIX="$HOME/.local/bin"` with the same URL.

**From source:**

```bash
git clone https://github.com/abdelrahman30x/TunnelBypass.git && cd TunnelBypass
go build -trimpath -ldflags "-s -w" -o tunnelbypass ./cmd
./tunnelbypass
```

### Windows

```powershell
irm https://raw.githubusercontent.com/abdelrahman30x/TunnelBypass/main/scripts/install.ps1 | iex
```

If execution policy blocks: `Set-ExecutionPolicy -Scope Process -ExecutionPolicy Bypass`, then run again.

Build from source: same `go build` as above, then `.\tunnelbypass.exe`.

### Commands

| What | Command |
| :--- | :--- |
| Interactive | `tunnelbypass` |
| Run (see flags) | `tunnelbypass run -help` |
| Configs only | `tunnelbypass generate …` |
| Uninstall | `tunnelbypass uninstall` |
| Status | `tunnelbypass status` / `health` |

Example: `./tunnelbypass run portable reality --port 443 --sni example.com --uuid auto`

---

## Video

[![Tutorial](https://img.youtube.com/vi/praPwuKj9d4/maxresdefault.jpg)](https://www.youtube.com/watch?v=praPwuKj9d4)

**[TunnelBypass — setup (YouTube)](https://www.youtube.com/watch?v=praPwuKj9d4)**

---

## Notes

- Some steps need **administrator / root** (firewall, services). Use `--no-elevate` or `run portable` otherwise.
- **`--portable`**, **`--data-dir`**, **`TUNNELBYPASS_DATA_DIR`** choose where configs and logs live (including Docker).

**Repo:** [TunnelBypass](https://github.com/abdelrahman30x/TunnelBypass) · [Releases](https://github.com/abdelrahman30x/TunnelBypass/releases)  
Scripts: [`scripts/install.sh`](scripts/install.sh), [`scripts/install.ps1`](scripts/install.ps1)

---

## Docker

```bash
docker build -t tunnelbypass:local .
docker run -it --rm -v tbdata:/data tunnelbypass:local
```

[docker-compose.yml](docker-compose.yml), [docker/env.example](docker/env.example)

---

## Docs & data paths

More: **[docs/wiki/](docs/wiki/README.md)**  
**Arabic (for beginners):** [لماذا هذا البرنامج؟](docs/wiki/Home.md#arabic-beginners) — DNS، تحسين تلقائي، وسياق DPI باختصار.

| OS | Default system-wide base |
| :--- | :--- |
| Windows | `C:\TunnelBypass\` |
| Linux | `/usr/local/etc/tunnelbypass/` |

Under the data root: `configs/<protocol>/`, `logs/`, `run/`.

---

## Clients

v2rayN / Nekoray / sing-box; WireGuard apps; SSH clients where applicable. Self-signed TLS: allow insecure or pin certs as generated.

---

## Repository layout

`cmd/` → `internal/cli` · `core/provision` · `core/installer` · `core/transports/*` · `tools/host_catalog`

---

## Acknowledgments

Parts of this project were developed with assistance from **[Cursor](https://cursor.com)** and **Google Gemini**.

---

## License

[MIT](LICENSE).
