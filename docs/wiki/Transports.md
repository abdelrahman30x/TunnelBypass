# Transports and stealth

[← Handbook](README.md)

## How TunnelBypass models transports

TunnelBypass **writes configs** under a predictable tree (`configs/<transport>/`, logs beside them) and can install **one OS service** per flow when you ask for it. Choosing a transport means picking a **recipe**: the interactive flow or `tunnelbypass run …` with the right flags.

## Stealth ladder (rule of thumb)

Stricter networks often tolerate traffic that **looks like ordinary HTTPS** to a real hostname more than raw or obvious proxy shapes. A coarse ordering (see also the [root README](../../README.md) table):

1. **VLESS + REALITY (TCP / Vision)** — strong TLS camouflage when tuned with a plausible SNI.
2. **VLESS + REALITY + gRPC** — HTTP/2 framing; REALITY + gRPC is the supported gRPC path here (see project notes in code).
3. **WebSocket / WSS paths** — useful where HTTP upgrades are normal; different trade-offs than QUIC.
4. **QUIC / Hysteria** — excellent when UDP is not throttled; weaker when UDP is degraded.

This is **not** a guarantee against every DPI stack—only a practical ordering for many ISP conditions.

## Engines and clients

| Engine family | Typical client side |
|---------------|---------------------|
| Xray (VLESS, WSS, …) | v2rayN, Nekoray, sing-box, etc. |
| Hysteria v2 | Official / compatible Hy2 clients |
| WireGuard | OS WireGuard apps |
| SSH / stunnel / wstunnel | SSH clients, custom HTTP stacks |

Self-signed certificates: clients may need **allow insecure** or pinned certs—match what the tool generated.

## Spec files and non-interactive runs

For automation, prefer **`--spec`** / config flags documented in `tunnelbypass run -help` so CI or config management can call the same paths as the interactive flow without a TTY.
