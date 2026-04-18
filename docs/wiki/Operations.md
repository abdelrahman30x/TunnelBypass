# Operations

[← Handbook](README.md)

## Everyday commands

| Intent | Entry point |
|--------|-------------|
| Interactive setup | `tunnelbypass` (no args) |
| Non-interactive run | `tunnelbypass run …` — see `-help` |
| Config only | `tunnelbypass generate …` |
| Remove service + transport files | `tunnelbypass uninstall …` |
| Quick environment / probes | `tunnelbypass status` / `tunnelbypass health` |
| Dependency graph | `tunnelbypass deps-tree [--mermaid] <transport>` |

## Health and “ping check”

When available, **`status`** / **`health`** can summarize data dir, listeners, and optional **ping-style checks** (implementation in `internal/health`). Use this to separate **server-to-internet** issues from **client path** issues.

## Portable mode

`run portable …` keeps everything in the foreground and avoids elevating for a system service when your transport allows it—useful for laptops, CI, or Docker without systemd enable.

<a id="lan-self-host"></a>

## LAN self-host mode

Use this when **both ends are on the same LAN** and clients should connect to a **private (RFC1918) IP** instead of the machine’s public address.

**Interactive wizard:** `tunnelbypass --self-host` (no command) opens the main menu with LAN mode enabled for setup (no public-IP step). Same with `tunnelbypass --self-host wizard` or `tunnelbypass wizard --self-host` after root `flag` parsing.

**CLI `run` / `generate`:**

- `tunnelbypass run --self-host …`
- `tunnelbypass generate --self-host …` (same flags as `run`; implies config-only)
- `tunnelbypass --self-host run …` — top-level `--self-host` / `--lan-relax` are forwarded into `run` when they appear **before** the `run` token.

**`run` flag ordering:** The Go `flag` package stops at the first non-flag argument. Prefer **`… --port N --sni host reality`** (transport **last**). Current releases also **reorder** argv when a single known transport appears **before** trailing flags (e.g. `… self-host reality --port 8443` → flags then `reality`).

### Flags and environment

| Flag / env | Role |
|------------|------|
| **`--self-host`** | Enable LAN mode: resolve endpoint once (logged under `[network]`), write that IP into specs and generated client artifacts. |
| **`--lan-relax`** | With `--self-host`: **Relaxed LAN** — omit Xray’s `geoip:private` block rule so tunnel traffic can reach other LAN hosts. **Default without this flag:** **Strict LAN** (block private pivoting). |
| **`--server <ip>`** | Highest priority: use this address as the endpoint (still with `--self-host` behavior for routing policy and summaries). |
| **`TUNNELBYPASS_LAN_IP`** | Second priority after `--server`: force the LAN IP without auto-detection. Must be a valid IP literal. |
| *(auto)* | Otherwise: default-route interface → ranked interface scan → **`127.0.0.1`** if nothing matches (single-machine / container). |

A mandatory **`⚠️ LAN SELF-HOST MODE ACTIVE`** line is printed on **stderr** every time.

### Resolution and safety

- Resolution runs **once** in the engine, then **`host_mode` / `lan_relax`** in JSON behave like the CLI (see `internal/cfg` / `internal/network`).
- **`cfg.FillDefaults`** does **not** replace the server address with a public IP when `host_mode` is `lan`.
- **TCP reachability** after provision is **advisory** (stderr warning if the probe fails). **UDP-only** transports (**`hysteria`**, **`wireguard`**) skip the TCP check.

### Debugging

- **`tunnelbypass run --debug --self-host …`** enables a verbose `[network]` trace (priorities, interface choice, exclusions).
- Combine with **`--dry-run`** to generate configs without starting services.

## Debugging

- `tunnelbypass --debug` for verbose logs where supported (for **`run`**, you can also pass **`--debug`** on the `run` flag set so it applies before provisioning).
- Re-run with **`--dry-run`** when you want configs without touching services (see `run -help`).

## Unattended uninstall

Use **`--yes`** when stdin is not a TTY so scripts do not block on prompts.
