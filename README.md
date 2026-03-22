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

Use `make` or `go build` directly. Requires **Go 1.25+** (`go.mod`).

To embed the current version (e.g. `v1.2.0`), add `-ldflags "-X main.Version=v1.2.0"`.

| Scenario | Command |
|----------|---------|
| Linux/macOS — native binary | `go build -trimpath -ldflags "-s -w -X main.Version=$(git describe --tags --abbrev=0 || echo v0.0.0)" -o tunnelbypass ./cmd` |
| Linux/macOS — cross-compile Windows | `GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "-s -w -X main.Version=$(git describe --tags --abbrev=0 || echo v0.0.0)" -o tunnelbypass.exe ./cmd` <br> (or `make build-windows`) |
| Windows (PowerShell) — native binary | `go build -trimpath -ldflags "-s -w -X main.Version=$(git describe --tags --abbrev=0 || echo v0.0.0)" -o tunnelbypass.exe ./cmd` <br> (or `.\scripts\build.ps1`) |
| Windows (PowerShell) — cross-compile Linux | `$env:GOOS='linux'; $env:GOARCH='amd64'; go build -trimpath -ldflags "-s -w -X main.Version=$(git describe --tags --abbrev=0 || echo v0.0.0)" -o tunnelbypass ./cmd` |

Note: `$(git describe --tags --abbrev=0 || echo v0.0.0)` sets the version from the latest Git tag, or `v0.0.0` if no tags exist.


## Elevation and portable mode

**Note:** UAC/sudo may re-exec for elevation; `TB_PORTABLE`, `TB_DATA_DIR`, Docker, and service installs affect layout and permissions.

- Non-portable `run` for service-capable transports may elevate once so WinSW/systemd and firewall rules can run; use `--no-elevate`, `TB_NO_ELEVATE=1`, or `run portable` to stay in the current session.
- `TB_PORTABLE=1` and `TB_DATA_DIR` select a user-writable data layout.

## Direct download (GitHub releases)

Repo: [https://github.com/abdelrahman30x/TunnelBypass](https://github.com/abdelrahman30x/TunnelBypass) — [latest release](https://github.com/abdelrahman30x/TunnelBypass/releases/latest).

Release asset names vary by pipeline; adjust the filters below if your archive uses different patterns (e.g. `.zip` — unpack after download). Requires a published release with binaries attached.

### Suggested Release Asset Naming

For a `v1.2.0` release (or `$(git describe --tags --abbrev=0 || echo v0.0.0)` for the actual version):

| OS | Suggested Filename | Contents |
|----|--------------------|----------|
| Linux (amd64) | `tunnelbypass_v1.2.0_linux_amd64.tar.gz` | `tunnelbypass` (executable) |
| Windows (amd64) | `tunnelbypass_v1.2.0_windows_amd64.exe` | `tunnelbypass.exe` (executable) |

Example `gh release` command to create a release and upload assets:

```bash
VERSION=$(git describe --tags --abbrev=0 || echo v0.0.0)
gh release create $VERSION \
  tunnelbypass_${VERSION}_linux_amd64.tar.gz#"TunnelBypass for Linux (AMD64)" \
  tunnelbypass_${VERSION}_windows_amd64.exe#"TunnelBypass for Windows (AMD64)" \
  --title "Release $VERSION" --notes "See CHANGELOG.md for details."
```

### Windows (PowerShell)

```powershell
$owner = "abdelrahman30x"; $repo = "TunnelBypass"
$rel = Invoke-RestMethod -Uri "https://api.github.com/repos/$owner/$repo/releases/latest" `
  -Headers @{ "User-Agent" = "TunnelBypass-Setup" }
$asset = $rel.assets | Where-Object { $_.name -match '\.(exe|zip)$' -and $_.name -match 'windows|win|\.exe$' } | Select-Object -First 1
if (-not $asset) { $asset = $rel.assets | Where-Object { $_.name -like "*.exe" } | Select-Object -First 1 }
if (-not $asset) { throw "No matching Windows asset in the latest release. Download manually from GitHub Releases." }
Invoke-WebRequest -Uri $asset.browser_download_url -OutFile $asset.name
if ($asset.name -match '\.zip$') {
  Expand-Archive -Path $asset.name -DestinationPath . -Force
  Get-ChildItem -Filter "tunnelbypass*.exe" -Recurse -ErrorAction SilentlyContinue | Select-Object -First 1 | ForEach-Object { Copy-Item $_.FullName -Destination ".\tunnelbypass.exe" -Force }
} elseif ($asset.name -match '\.exe$') {
  Copy-Item $asset.name -Destination ".\tunnelbypass.exe" -Force
}
.\tunnelbypass.exe --version
```

### Linux (amd64, bash + [jq](https://jqlang.github.io/jq/))

```bash
OWNER=abdelrahman30x REPO=TunnelBypass
JSON=$(curl -fsSL -H "Accept: application/vnd.github+json" -H "User-Agent: tunnelbypass" \
  "https://api.github.com/repos/${OWNER}/${REPO}/releases/latest")
URL=$(echo "$JSON" | jq -r '.assets[] | select(.name | test("linux.*(amd64|x86_64)"; "i")) | .browser_download_url' | head -1)
test -n "$URL" || { echo "No linux/amd64 asset found; pick one from the releases page." >&2; exit 1; }
curl -fsSL -o tb-download "$URL"
case "$URL" in
  *.tar.gz|*.tgz) tar -xzf tb-download && rm -f tb-download ;;
  *.zip) unzip -o tb-download && rm -f tb-download ;;
  *) mv tb-download tunnelbypass ;;
esac
chmod +x tunnelbypass 2>/dev/null || true
./tunnelbypass --version
```

### macOS (bash + jq)

```bash
OWNER=abdelrahman30x REPO=TunnelBypass
ARCH=$(uname -m)
case "$ARCH" in
  arm64) PAT='darwin.*(arm64|aarch64)|apple.*silicon';;
  *)     PAT='darwin.*(amd64|x86_64)';;
esac
JSON=$(curl -fsSL -H "Accept: application/vnd.github+json" -H "User-Agent: tunnelbypass" \
  "https://api.github.com/repos/${OWNER}/${REPO}/releases/latest")
URL=$(echo "$JSON" | jq -r --arg pat "$PAT" '.assets[] | select(.name | test($pat; "i")) | .browser_download_url' | head -1)
test -n "$URL" || { echo "No macOS asset for $ARCH; download manually from GitHub Releases." >&2; exit 1; }
curl -fsSL -o tb-download "$URL"
case "$URL" in
  *.tar.gz|*.tgz) tar -xzf tb-download && rm -f tb-download ;;
  *.zip) unzip -o tb-download && rm -f tb-download ;;
  *) mv tb-download tunnelbypass ;;
esac
chmod +x tunnelbypass
./tunnelbypass --version
```

### GitHub CLI (optional)

```bash
gh release download --repo abdelrahman30x/TunnelBypass --clobber
```

Then run the binary that matches your OS from the current directory.

### Example `run` commands (non-interactive)

Wizard order and rough DPI-bypass strength (more stars ≈ stronger camouflage on hostile networks). On Windows use `.\tunnelbypass.exe` instead of `./tunnelbypass`. Add `--data-dir <path>` or `TB_PORTABLE=1` when you need a fixed or user-writable data root.

| | Stack | Strength | Example |
|---|--------|----------|---------|
| 1 | Reality / XTLS | ★★★★★ | `./tunnelbypass run --type reality --port 443 --sni epicgames.com --uuid auto` |
| 2 | WSS (wstunnel) | ★★★★ | `./tunnelbypass run --type wss --port 443 --sni epicgames.com` |
| 3 | TLS (stunnel) | ★★★ | `./tunnelbypass run --type tls --port 443 --sni epicgames.com` |
| 4 | QUIC (Hysteria v2) | ★★ | `./tunnelbypass run --type hysteria --port 443 --sni epicgames.com --uuid auto` |
| 5 | SSH | ★★ | `./tunnelbypass run ssh` |
| 6 | WireGuard | ★ | `./tunnelbypass run --type wireguard --port 51820 --sni example.com` |
|   |           |   |                                                                  |

**Portable Mode** (No elevation, no service install, same session):
`./tunnelbypass run portable <any_transport>`

## Docker

Image uses non-root user, `TB_DATA_DIR=/data`, `TB_PORTABLE=1`, default `TB_LOG=json`. Map ports your transport needs.

**Note:** Prefer `docker run ... tunnelbypass:local run <transport>`; wizard service installs are unreliable in containers.

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

**Portable / CI:** use `TB_LOG=json` for structured logs; see `tunnelbypass run -help` for flags, exit behavior, dependency order, and `--install-service`.

```bash
tunnelbypass run ssh
tunnelbypass run --daemon ssh
tunnelbypass run portable <transport> # execute directly, skipping service/UAC elevation
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

## License

[MIT](LICENSE).
