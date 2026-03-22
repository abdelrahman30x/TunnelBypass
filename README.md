# TunnelBypass

CLI to install and run tunnel stacks on **Windows** and **Linux**: VLESS (Reality), Hysteria v2, WireGuard, SSH / stunnel / wstunnel. Interactive wizard, dependency downloads, firewall hints, and **systemd** / **WinSW** (or **user-mode**) supervision. Configs and keys live under a data directory.

Run it on the host that terminates the tunnel, then import generated client configs or URLs on phones/desktops.

## Philosophy

- Small setup surface: wizard, generated configs, optional OS services.
- Go orchestrates and writes configs; protocol throughput is in external binaries (Xray, Hysteria, etc.).
- Useful on restrictive networks (TLS/SNI camouflage, several stack shapes).

## Quickstart

```bash
go build -o tunnelbypass ./cmd
./tunnelbypass --version
./tunnelbypass
```

Non-interactive example: `TB_PORTABLE=1 ./tunnelbypass run ssh` (see **Run** below).

## Build

```bash
make build
# or
go build -o tunnelbypass ./cmd    # Unix
go build -o tunnelbypass.exe ./cmd # Windows
```

On Windows without `make`, use `scripts/build.ps1`. Requires **Go 1.25+** (`go.mod`).

## Elevation and portable mode

**Note:** UAC/sudo re-exec, `TB_PORTABLE`, `TB_DATA_DIR`, Docker, and service installs are documented in [STABILITY.md](STABILITY.md). Prefer that file for edge cases instead of duplicating detail here.

- Non-portable `run` for service-capable transports may elevate once so WinSW/systemd and firewall rules can run; use `--no-elevate`, `TB_NO_ELEVATE=1`, or `run portable` to stay in the current session.
- `TB_PORTABLE=1` and `TB_DATA_DIR` select user-writable layout (see STABILITY for full env table).

## Direct download (releases)

Repo: [https://github.com/abdelrahman30x/TunnelBypass](https://github.com/abdelrahman30x/TunnelBypass)

**Windows (PowerShell):** fetch latest `.exe` from the GitHub API, then:

```powershell
.\tunnelbypass.exe --version
.\tunnelbypass.exe run --type reality --port 443 --sni epicgames.com --uuid auto
```

**Linux:** same idea with `curl`/`python3` to pick a linux/amd64 asset; add `--data-dir` for a fixed data root if needed.

## Docker

Image uses non-root user, `TB_DATA_DIR=/data`, `TB_PORTABLE=1`, default `TB_LOG=json`. Map ports your transport needs.

**Note:** Prefer `docker run ... tunnelbypass:local run <transport>`; wizard service installs are unreliable in containers. Details: [STABILITY.md](STABILITY.md).

```bash
docker build -t tunnelbypass:local .
docker run -it --rm -v tbdata:/data tunnelbypass:local
docker run --rm -v tbdata:/data tunnelbypass:local run hysteria
```

See [docker-compose.yml](docker-compose.yml), [docker/env.example](docker/env.example).

## Run

```text
tunnelbypass              # wizard
tunnelbypass run -help     # portable / flags
tunnelbypass generate      # configs only
tunnelbypass uninstall
tunnelbypass status | health
tunnelbypass deps-tree [--mermaid] <transport>
tunnelbypass xray-svc | hysteria-svc | udpgw-svc   # service helpers
```

**Portable / CI:** `TB_LOG=json`, exit codes, dependency order, and `--install-service` limits are summarized in [STABILITY.md](STABILITY.md).

```bash
tunnelbypass run ssh
tunnelbypass run --daemon ssh
tunnelbypass run --data-dir /data --type reality --port 443 --sni example.com --uuid auto --dry-run
```

Default data root without `--config`/`--spec`: `%LOCALAPPDATA%\TunnelBypass` / `~/.local/share/tunnelbypass`. Use `--system-data` or `--config` JSON for system-wide paths (see **Data dirs**).

## Uninstall, generate, deps-tree

| Command | Purpose |
|---------|---------|
| `status` / `health` | Data dir, listen ports, user-supervisor PIDs |
| `uninstall` | Remove managed OS service + that transport’s configs under the data root |
| `generate` | Same flags as `run`, configs only (no listener) |
| `deps-tree` | Dependency graph; `--mermaid` for a diagram |

`tunnelbypass uninstall --type reality --data-dir /data --yes` — **`--yes`** required when stdin is not a TTY.

## Troubleshooting

```bash
tunnelbypass --debug wizard
TB_DEBUG=1 tunnelbypass wizard
TB_LOG=json tunnelbypass run ssh
```

## Layout

- `cmd/` — binary (`internal/cli`)
- `core/provision` — config generation for `run`
- `internal/cfg` — `RunSpec`, `--config`, `--spec`
- `core/installer` — paths, downloads, TLS, services, SSH stack
- `core/svcman` — systemd / WinSW / user supervisor
- `core/udpgw` — UDPGW (internal + optional external binary)
- `core/ssh` — embedded SSH when system OpenSSH is missing
- `core/binmgr` — optional SHA-256 manifests (`checksums.json`)
- `core/transports/*` — per-protocol config writers
- `internal/*` — helpers (`runtimeenv`, `tblog`, …)
- `tools/host_catalog` — SNI / bug-host data

## Data dirs

| OS | System-wide base |
|----|------------------|
| Windows | `C:\TunnelBypass\` |
| Linux | `/usr/local/etc/tunnelbypass/` |

Default `run` uses the per-user portable-style base unless `--system-data` or `paths.data_dir` in `--config`. Under base: `configs/<protocol>/`, `logs/`, `run/`.

## Clients

v2rayN / NekoBox / Sing-Box (VLESS, Hysteria); WireGuard app (`.conf`); SSH clients for plain / TLS / WSS. Self-signed TLS paths: allow insecure in the client.

## Project docs

| Doc | Purpose |
|-----|---------|
| [CHANGELOG.md](CHANGELOG.md) | Releases |
| [VERSIONING.md](VERSIONING.md) | Semver |
| [STABILITY.md](STABILITY.md) | Env vars, limits, elevation, containers |

## License

[MIT](LICENSE).
