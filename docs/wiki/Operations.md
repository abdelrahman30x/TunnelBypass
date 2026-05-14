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

`run portable …` keeps everything in the foreground and avoids elevating for a system service when your transport allows it—useful for laptops or CI.

## Debugging

- `tunnelbypass --debug` for verbose logs where supported (for **`run`**, you can also pass **`--debug`** on the `run` flag set so it applies before provisioning).
- Re-run with **`--dry-run`** when you want configs without touching services (see `run -help`).
- **Client TLS errors toward public sites** (e.g. sing-box + `shadowsocks` outbound dialing `*.whatsapp.com`) are almost always **client trust / DNS / rules**, not something fixed by “server allowInsecure.” See [Transports — TLS / allowInsecure](Transports.md#tls-allowinsecure).
- **`inbound/mixed[mixed-in]: … x509: unknown authority`** with **Shadowsocks + v2ray-plugin** usually means the **tunnel’s self-signed cert** is still being verified — fix **plugin_opts / allowInsecure** on the client (Throne, NekoRay, etc.), not the VPS. See [Transports — sing-box / Throne / NekoRay](Transports.md#sing-box-client-tls).

## Unattended uninstall

Use **`--yes`** when stdin is not a TTY so scripts do not block on prompts.
