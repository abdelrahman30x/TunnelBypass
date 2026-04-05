# Linux networking (transit)

[← Handbook](README.md)

Applies when TunnelBypass applies **Linux transit** options: sysctl drop-ins, iptables chains, optional DNS/`gai` helpers. Read **`core/installer/linux_transit.go`** top-of-file philosophy comment for the authoritative behavior.

## What gets tuned

- **MSS clamping** — custom `mangle` chain (e.g. `TUNNEL_BYPASS`) with `TCPMSS --clamp-mss-to-pmtu`, jumped from `FORWARD` and `OUTPUT`, intended to reduce fragmentation pain without forcing a single global MTU on clients.
- **Kernel parameters** — drop-in under `/etc/sysctl.d/` (not editing `/etc/sysctl.conf` directly), including congestion control / queue discipline where enabled, and **socket buffer limits** relevant to QUIC/UDP throughput.
- **Autopilot** — optional probes (jitter / latency) can flip **optimize** paths before privileged install—see `linux_autopilot.go` and engine integration.

## Safety: “last standing” rollback

Global rollback (sysctl / iptables cleanup) should not strip settings still needed by **another** TunnelBypass service instance on the same host. The codebase uses a **remaining unit count** style check so partial uninstall does not break other inbounds.

## Environment knobs

Documented in code and root README where relevant: `TUNNELBYPASS_LINUX_GAI`, optional watcher notes, etc. Prefer **application-level DNS inside Xray** (`MergeXrayDNSIntoConfig`) before forcing system-wide resolver rewrites.

## Operational caution

sysctl and firewall changes affect **the whole machine**. Test on a non-production host when possible; keep backups and a plan to undo changes (see [Operations](Operations.md) for uninstall and service behavior).
