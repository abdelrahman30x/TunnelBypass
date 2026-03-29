# TunnelBypass

**What it is:** A command-line tool that sets up VPN-style tunnels on a server (Windows or Linux): it can walk you through setup, generate configs, download helpers (Xray, Hysteria, etc.), and optionally install OS services. You run it on the machine that **ends** the tunnel; clients import the generated configs or links.

**What it is not:** It is not the tunnel protocol itself—throughput is handled by the bundled/external binaries you choose (VLESS/Reality, Hysteria, WireGuard, SSH / stunnel / wstunnel, …).

---

## 📺 Video Tutorial

[![TunnelBypass Tutorial](https://img.youtube.com/vi/praPwuKj9d4/maxresdefault.jpg)](https://www.youtube.com/watch?v=praPwuKj9d4)

> **Watch:** [TunnelBypass — Full Setup Tutorial](https://www.youtube.com/watch?v=praPwuKj9d4)

---

## Quickstart (from source)

**Go 1.25+** — see `go.mod`. From the repository root:

```bash
go build -trimpath -ldflags "-s -w" -o tunnelbypass ./cmd
./tunnelbypass --version
./tunnelbypass
```

Optional: set the reported version with e.g. `-ldflags "-s -w -X main.Version=v1.2.1"` (same `go build` line, extend the flags).

Non-interactive example: `./tunnelbypass run portable ssh` or `./tunnelbypass run --data-dir <path> ssh` (see **Run** below).

## Elevation & data layout (short)

- Some `run` modes ask for **admin/root** once (service + firewall). Use `--no-elevate` or `run portable` to stay in the current user session.
- **`--portable`** / **`--data-dir`** / Docker **`TUNNELBYPASS_DATA_DIR`** choose where configs and logs live.

## Direct download (GitHub releases)

Repo: [TunnelBypass](https://github.com/abdelrahman30x/TunnelBypass) — [latest release](https://github.com/abdelrahman30x/TunnelBypass/releases/latest).

### Install (recommended)

**Linux / macOS**

```bash
curl -fsSL https://raw.githubusercontent.com/abdelrahman30x/TunnelBypass/main/scripts/install.sh | bash
```

Optional: install into a directory on `PATH`:

```bash
INSTALL_PREFIX="$HOME/.local/bin" curl -fsSL https://raw.githubusercontent.com/abdelrahman30x/TunnelBypass/main/scripts/install.sh | bash
```

**Windows (PowerShell)**

```powershell
irm https://raw.githubusercontent.com/abdelrahman30x/TunnelBypass/main/scripts/install.ps1 | iex
```

If execution policy blocks: `Set-ExecutionPolicy -Scope Process -ExecutionPolicy Bypass`, then run the same `irm ... | iex` again.

Optional folder:

```powershell
$env:INSTALL_PREFIX = "$env:LOCALAPPDATA\Programs\TunnelBypass"
irm https://raw.githubusercontent.com/abdelrahman30x/TunnelBypass/main/scripts/install.ps1 | iex
```

Source: [`scripts/install.sh`](scripts/install.sh), [`scripts/install.ps1`](scripts/install.ps1) — they resolve **latest** release, OS/arch, unpack, and place the binary; optional env: `INSTALL_PREFIX`, `INSTALL_VERSION` (pin a tag), `INSTALL_OWNER` / `INSTALL_REPO` (forks).

## Docker

Image sets **`TUNNELBYPASS_DATA_DIR=/data`**. Map whatever ports your transport uses. For servers in containers, prefer `docker run ... tunnelbypass:local run <transport>` (wizard “install service” is a poor fit inside Docker).

```bash
docker build -t tunnelbypass:local .
docker run -it --rm -v tbdata:/data tunnelbypass:local
docker run --rm -v tbdata:/data tunnelbypass:local run hysteria
```

[docker-compose.yml](docker-compose.yml), [docker/env.example](docker/env.example).

## Run

Non-interactive examples, ordered by how well they blend in on **strict / filtered networks** (more ★ = stronger camouflage). On Windows, use `.\tunnelbypass.exe` instead of `./tunnelbypass`. Add `--data-dir <path>` or `run portable` when you want a user-writable data folder.

| Protocol | Camouflage | One-liner |
| :-- | :-: | :-- |
| **Reality / XTLS** | ★★★★★ | `./tunnelbypass run --type reality --port 443 --sni epicgames.com --uuid auto` |
| **WSS** (wstunnel) | ★★★★ | `./tunnelbypass run --type wss --port 443 --sni epicgames.com` |
| **TLS** (stunnel) | ★★★ | `./tunnelbypass run --type tls --port 443 --sni epicgames.com` |
| **Hysteria v2** (QUIC) | ★★ | `./tunnelbypass run --type hysteria --port 443 --sni epicgames.com --uuid auto` |
| **SSH** | ★★ | `./tunnelbypass run ssh` |
| **WireGuard** | ★ | `./tunnelbypass run --type wireguard --port 51820 --sni example.com` |

**Portable run** (foreground, same session, no OS service install):  
`./tunnelbypass run portable <transport> [flags]`

```text
tunnelbypass              # wizard
tunnelbypass run -help     # all run flags
tunnelbypass generate      # configs only
tunnelbypass uninstall
tunnelbypass status | health
tunnelbypass deps-tree [--mermaid] <transport>
tunnelbypass xray-svc | hysteria-svc | udpgw-svc   # service helpers
```

Full details: `tunnelbypass run -help` (dependencies, `--install-service`, exit behavior).

```bash
tunnelbypass run ssh
tunnelbypass run --daemon ssh
tunnelbypass run portable reality --port 443 --sni example.com --uuid auto
tunnelbypass run portable ssh
tunnelbypass run --data-dir /data --type reality --port 443 --sni example.com --uuid auto --dry-run
```

Default data root without `--config` / `--spec`: `%LOCALAPPDATA%\TunnelBypass` / `~/.local/share/tunnelbypass`. Use `--system-data` or `--config` for system-wide paths (see **Data dirs**).

## Uninstall, generate, deps-tree

| Command | Purpose |
|---------|---------|
| `status` / `health` | Data dir, listen ports, user-supervisor PIDs |
| `uninstall` | Remove managed OS service + that transport’s configs under the data root |
| `generate` | Same flags as `run`, configs only (no listener) |
| `deps-tree` | Dependency graph; `--mermaid` for a diagram |

`tunnelbypass uninstall --type reality --data-dir /data --yes` — **`--yes`** if stdin is not a TTY.

## Troubleshooting

```bash
tunnelbypass --debug wizard
tunnelbypass run -help
```

## Repository layout (for contributors)

- `cmd/` — entry binary → `internal/cli`
- `core/provision` — config generation for `run`
- `internal/cfg` — `RunSpec`, `--config`, `--spec`
- `core/installer` — paths, downloads, TLS, services, SSH stack
- `core/svcman` — systemd / WinSW / user supervisor
- `core/udpgw` — UDPGW
- `core/ssh` — embedded SSH when system OpenSSH is missing
- `core/binmgr` — optional SHA-256 manifests (`checksums.json`)
- `core/transports/*` — per-protocol config writers
- `internal/*` — helpers (`runtimeenv`, `tblog`, …)
- `tools/host_catalog` — SNI / host data

## Data dirs

| OS | System-wide base |
|----|------------------|
| Windows | `C:\TunnelBypass\` |
| Linux | `/usr/local/etc/tunnelbypass/` |

Default `run` uses the per-user base unless `--system-data` or `paths.data_dir` in `--config`. Under base: `configs/<protocol>/`, `logs/`, `run/`.

## Clients

v2rayN / NekoBox / Sing-Box (VLESS, Hysteria); WireGuard app; SSH clients for plain / TLS / WSS. Self-signed TLS: allow insecure in the client if you used self-signed certs.

## License

[MIT](LICENSE).
