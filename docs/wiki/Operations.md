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

## Debugging

- `tunnelbypass --debug` for verbose logs where supported.
- Re-run with **`--dry-run`** when you want configs without touching services (see `run -help`).

## Unattended uninstall

Use **`--yes`** when stdin is not a TTY so scripts do not block on prompts.
