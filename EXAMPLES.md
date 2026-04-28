# Examples

Copy, paste, change the domain/IP.

---

## Wizard

```bash
tunnelbypass
```

---

## VLESS + REALITY

```bash
tunnelbypass run reality --port 443 --sni www.example.com --uuid auto
```

With system service:
```bash
tunnelbypass run reality --port 443 --sni www.example.com --uuid auto --auto-start
```

Foreground only:
```bash
tunnelbypass run portable reality --port 443 --sni www.example.com --uuid auto
```

---

## VLESS + WebSocket + TLS

```bash
tunnelbypass run vless-ws --port 443 --sni ws.example.com --uuid auto
```

---

## VLESS + REALITY + gRPC

```bash
tunnelbypass run vless-grpc --port 443 --sni grpc.example.com --uuid auto
```

---

## Hysteria 2

```bash
tunnelbypass run hysteria --port 8443 --sni hy.example.com --uuid auto
```

---

## SSH

```bash
tunnelbypass run ssh --port 2222 --ssh-user tunnelbypass --ssh-password auto
```

With UDPGW:
```bash
tunnelbypass run portable ssh --port 2222 --ssh-user tunnelbypass --ssh-password auto --udpgw-port 7300
```

---

## SSH-TLS

```bash
tunnelbypass run ssh-tls --port 2053 --sni ssh-tls.example.com --ssh-user tunnelbypass
```

---

## WSS

```bash
tunnelbypass run wss --port 443 --sni wss.example.com
```

---

## TLS (stunnel-style)

```bash
tunnelbypass run tls --port 443 --sni tls.example.com
```

---

## Config only

```bash
tunnelbypass generate reality --port 443 --sni example.com --uuid auto
tunnelbypass generate hysteria --port 8443 --sni example.com --uuid auto
```

---

## LAN self-host

Interactive:
```bash
tunnelbypass --self-host
```

One-shot:
```bash
tunnelbypass --self-host run reality --port 8443 --sni lan.local
```

Override IP:
```bash
TUNNELBYPASS_LAN_IP=192.168.1.10 tunnelbypass --self-host run reality --port 8443
```

Relaxed LAN (home lab only):
```bash
tunnelbypass --self-host --lan-relax run reality --port 8443
```

---

## Portable / custom data dir

```bash
tunnelbypass run portable reality --data-dir ./tb-data --port 443 --sni example.com
```

No elevation:
```bash
tunnelbypass run portable reality --no-elevate --port 443 --sni example.com
```

---

## Status & health

```bash
tunnelbypass status
tunnelbypass health
tunnelbypass uninstall
```

---

## Dry run

Generate configs without starting:
```bash
tunnelbypass run reality --dry-run --port 443 --sni example.com --uuid auto
```

---

## Daemon mode

Auto-restart on crash:
```bash
tunnelbypass run reality --daemon --port 443 --sni example.com --uuid auto
```

---

## From spec file

```bash
tunnelbypass run --spec myspec.yaml
```

---

## Full flags

```bash
tunnelbypass run -help
```
