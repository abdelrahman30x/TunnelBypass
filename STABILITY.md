# Stability, limitations, and known issues

**Note:** [README.md](README.md) summarizes behavior; portable mode, elevation, Docker, and env vars are spelled out here so we do not repeat long explanations in multiple docs.

This document is for operators and contributors. Behavior can change in minor releases; breaking changes are called out in [CHANGELOG.md](CHANGELOG.md) and version bumps per [VERSIONING.md](VERSIONING.md).

## What this tool does well

- One-machine setup: wizard, config files under a fixed base directory, and OS service registration for Xray / Hysteria / UDPGW-style helpers when running as root/admin.
- **Fallbacks**: user-mode supervision (PID + manifest under `<base>/run/`) when systemd / WinSW installation is not possible; optional **embedded SSH** when package OpenSSH cannot be installed (`TB_SSH_SERVER=auto|system|embed`).
- **Portable mode**: `tunnelbypass run --portable` or `TB_PORTABLE=1` uses a user-writable data directory and `bin/` layout for downloads.
- **SSH + UDPGW**: Use `tunnelbypass run --portable ssh` — internal UDPGW starts with embedded SSH. The separate `run udpgw` target is legacy only; `udpgw-svc` remains the OS-service entrypoint when the wizard installs SSH stacks.
- Regenerating JSON / WireGuard snippets from shared options (`core/types`).

## Limitations

- **Throughput and crypto** for VLESS / Reality / Hysteria are handled by **external binaries** (e.g. Xray-core), not by this Go codebase. This repo orchestrates install and config; it is not a replacement for tuning those programs.
- **Embedded SSH** ([`core/ssh/`](core/ssh/)) supports password authentication and **direct-tcpip** forwarding (typical for `ssh -L` / `-R`). It does **not** implement OpenSSH SOCKS `-D` inside the server; use system OpenSSH for that or client-side equivalents.
- **SSH / stunnel / wstunnel** flows still use external programs where applicable; unusual distros may need manual steps or embedded SSH + portable mode.
- **Windows** prefers WinSW for arbitrary executables when admin; without admin, services run as detached processes without the Windows Service Control Manager.
- **Auto-elevation on `run`:** Non-portable `run` for service-capable transports may request **UAC** / **`sudo`** once before provisioning (skipped in likely-containers, with `TB_PORTABLE=1`, or when **`--no-elevate`** / **`TB_NO_ELEVATE=1`** is set). If you decline elevation, the command fails with a short hint rather than exiting silently after a partial path.
- **Firewall messaging:** When not elevated, **`OpenFirewallPort`** skips `netsh`/Linux helpers and prints a **stderr** line explaining that the inbound rule was not added, instead of implying success.
- **Network reachability** (firewall, NAT, DNS, TLS inspection) is outside the tool; symptoms often look like “client cannot connect” while the local config is valid.
- **Checksums** for downloads are optional; add entries to [core/binmgr/checksums.json](core/binmgr/checksums.json) to enforce SHA-256 verification.

## Environment variables (selected)

| Variable | Purpose |
|----------|---------|
| `TB_PORTABLE=1` | User-writable base dir + portable `bin/` layout |
| `TB_DATA_DIR` | Absolute path as data root (Docker / scripts; implies portable layout checks for `run`); also skips wizard elevation so Docker menu option 1 works |
| `TB_SKIP_ELEVATE=1` | Skip admin/root elevation attempt before setup wizard (optional; `TB_DATA_DIR` implies this for Docker) |
| `TB_NO_ELEVATE=1` | Skip automatic UAC/`sudo` re-exec before non-portable `run` that installs OS services; use with **`--no-elevate`** for explicit CLI control |
| `TB_SSH_SERVER` | `auto` (default), `system` (package OpenSSH only), `embed` / `embedded` |
| `TB_SSH_LISTEN` | TCP port for embedded SSH (default `2222`) |
| `TB_UDPGW_MODE` | `auto`, `internal`, `external` |
| `TB_UDPGW_BINARY` | Path to external badvpn-udpgw-compatible binary (auto mode) |
| `TB_UDPGW_ARGS` | Extra CLI args for external UDPGW |
| `TB_LOG=json` | slog JSON to stdout |
| `TB_DEBUG=1` | Debug logs (slog + legacy debug) |

## Reliability & resilience

- **Child processes (Xray / Hysteria service wrapper):** Restarts use **exponential backoff** (from ~2s up to ~2m). After **12** consecutive **short** runs (under ~8 seconds) that end in error, the supervisor **stops restarting** to avoid tight crash loops. Set `TB_SVC_MAX_CRASH_LOOPS=0` for unlimited, or a positive integer to override. Logs go to `logs/TunnelBypass-Service.log` with **actionable hints** on common failures.
- **`tunnelbypass run --daemon`:** Uses the same backoff / crash-loop policy via `internal/supervisor`.
- **Downloads:** HTTP client timeout is **45 minutes** (large zips); failures include hints for DNS/network. Partial files are **removed** on copy failure.
- **UDPGW:** Default **512** concurrent TCP clients (`TB_UDPGW_MAX_CLIENTS`); idle TCP reads time out after **3 minutes** per client to limit stuck goroutines.
- **Embedded SSH:** Backend dials use a **20s** TCP connect timeout.
- **Config:** JSON configs are validated before starting Xray (syntax + empty file); YAML (Hysteria) is not fully parsed in advance.
- **Observability:** `tunnelbypass status` / `health` prints data dir, common ports, user-supervisor PID files — not a substitute for full monitoring.

## Edge cases

- **Port conflicts**: The wizard tries to allocate or reuse ports; two services binding the same port will still fail at runtime. UDPGW passes `-udpgw-port` to `udpgw-svc` when the wizard picks a non-default port.
- **Self-signed TLS** (SSH-over-TLS): Clients must allow insecure or install trust; documented in the README.
- **Partial install**: If a download fails mid-run, rerunning the wizard or clearing stale binaries under the install dir may be required.
- **UDPGW**: Internal server is badvpn-protocol compatible; optional external binary via `TB_UDPGW_*`. High load: tune OS buffers and prefer same host as the VPN client.
- **User-mode services**: Survive wizard exit as detached children; they are not restarted automatically on crash unless you use `tunnelbypass run --daemon` for that component.

## Reporting problems

When opening an issue, include OS, architecture, exact command, and with sensitive data redacted:

- Output with `TB_DEBUG=1` or `--debug` (see README).
- With `TB_LOG=json` if you need structured logs.
- Relevant lines from Xray / Hysteria logs under your data directory’s `logs/` if applicable.
