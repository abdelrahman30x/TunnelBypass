

<p align="center">
  <img src="https://github.com/user-attachments/assets/0a6cd2fa-10ae-43fd-9be1-46be294465bd" alt="Tunnel Bypass" width="100%">
</p>

<h1>Tunnel Bypass</h1>

<p align="center">
  <a href="https://github.com/abdelrahman30x/TunnelBypass/releases">
    <img src="https://img.shields.io/github/v/release/abdelrahman30x/TunnelBypass?style=flat-square&logo=github&color=181717" alt="Lanzamiento">
  </a>
  <a href="LICENSE">
    <img src="https://img.shields.io/badge/license-MIT-blue.svg?style=flat-square" alt="Licencia">
  </a>
  <a href="https://go.dev/dl/">
    <img src="https://img.shields.io/badge/Go-1.25+-00ADD8?logo=go&style=flat-square" alt="Go">
  </a>
</p>

<p>CLI de alto rendimiento para Xray, Hysteria y WireGuard. Despliega extremos de túnel resistentes a DPI en <strong>Linux</strong> o <strong>Windows</strong> con un solo comando. Configuraciones generadas automáticamente. Sin interfaz web: un solo binario.</p>

---

<h2>¿Qué es?</h2>

<p>TunnelBypass es una herramienta CLI que configura extremos de túnel en el lado del servidor. Te guía a través de un asistente interactivo, genera automáticamente configuraciones de cliente y enlaces para compartir, y puede instalarse opcionalmente como un servicio del sistema.</p>

<p>Construido en Go. Un solo binario. Sin dependencias.</p>

---

<h2>✨ Características</h2>

- 🧙 **Asistente interactivo** — ejecuta `tunnelbypass` y sigue las indicaciones
- 🔌 **Más de 8 transportes** incluidos
- ⚡ **Modo de ejecución única** — `tunnelbypass run <transport> ...` para scripts y automatización
- 📦 **Configuraciones y enlaces para compartir generados automáticamente** — listos para importar en v2rayN, Nekoray, sing-box, aplicaciones de WireGuard, etc.
- 🪟 **Integración opcional** con servicios de Windows / systemd de Linux
- 🔒 **Valores predeterminados seguros** con protección de red local

---

<h2>Transportes compatibles</h2>

<p><strong>VLESS / Xray</strong></p>

- `vless+reality` — TCP, XTLS Vision, se presenta como TLS real
- `vless+ws+tls` — WebSocket + TLS
- `vless+reality+grpc` — gRPC sobre REALITY

<p><strong>SSH y envoltorios</strong></p>

- `ssh` — SSH clásico; UDPGW opcional para UDP
- `ssh-tls` — Frontend TLS (Xray) con backend SSH
- `ssh-payload` — Escucha de carga HTTP para SSH estilo HTTP Custom / Netmod

<p><strong>UDP / QUIC</strong></p>

- `hysteria` — Hysteria 2 sobre QUIC
- `wireguard` — Túnel VPN estilo kernel
- `xdns` — VLESS + mKCP sobre UDP/53 (máscara DNS para censura extrema)

<p><strong>Otros</strong></p>

- `shadowsocks` — Shadowsocks 2022 (AEAD) mediante shadowsocks-rust
- `wss` — WebSocket + TLS mediante wstunnel
- `tls` — Envoltorio TLS (estilo stunnel)

<p>Ejecuta <code>tunnelbypass run -help</code> para ver todos los valores de <code>--type</code> y banderas.</p>

---

<h2>Instalación</h2>

<h3>Linux</h3>

```bash
curl -fsSL https://raw.githubusercontent.com/abdelrahman30x/TunnelBypass/main/scripts/install.sh | bash
```

Cuando se ejecuta como `root`, el instalador coloca `tunnelbypass` en `/usr/local/bin`.
Para usuarios no root, se instala en `$HOME/.local/bin`; si tu shell actual aún no ha cargado esa variable PATH, ejecuta exactamente el comando `Run now:` impreso por el instalador o abre una nueva terminal.

Prefijo personalizado opcional:
```bash
INSTALL_PREFIX="$HOME/.local/bin" curl -fsSL ... | bash
```

<h3>Windows</h3>

```powershell
irm https://raw.githubusercontent.com/abdelrahman30x/TunnelBypass/main/scripts/install.ps1 | iex
```

> Si la política de ejecución lo bloquea: `Set-ExecutionPolicy -Scope Process -ExecutionPolicy Bypass`, y vuelve a intentarlo.

<h3>Desde el código fuente</h3>

```bash
git clone https://github.com/abdelrahman30x/TunnelBypass.git && cd TunnelBypass
go build -trimpath -ldflags "-s -w" -o tunnelbypass ./cmd
```

Windows: mismo comando `go build`, luego `.\tunnelbypass.exe`.

---

<h2>Uso</h2>

<p><strong>Interactivo</strong></p>

```bash
tunnelbypass
```

<p><strong>Ejecución única</strong></p>

```bash
tunnelbypass run portable reality --port 443 --sni example.com --uuid auto
```

<p><strong>Solo generar configuraciones</strong></p>

```bash
tunnelbypass generate reality --port 443 --sni example.com
```

<p><strong>Otros comandos</strong></p>

```bash
tunnelbypass status      # estado en tiempo de ejecución / servicio
tunnelbypass health      # comprobación rápida de estado
tunnelbypass uninstall   # elimina configuraciones, servicios y binario
```

---

<h2>Ejemplos</h2>

<p>Consulta <a href="EXAMPLES.md">EXAMPLES.md</a> para comandos listos para copiar y pegar y archivos de especificación.</p>

---

<h2>Estructura del proyecto</h2>

```
cmd/         → punto de entrada
internal/    → CLI, motor, red, utilidades de configuración
core/        → aprovisionamiento, instalador, transportes, gestión de servicios
tools/       → utilidades para catálogo de hosts
docs/wiki/   → documentación completa (red, firewall, guía en árabe, etc.)
```

---

<h2>Consejos</h2>

- Algunos transportes requieren **root / administrador** (reglas de firewall, servicios). Usa `--no-elevate` o `run portable` para mantenerse en nivel de usuario.
- Modo portátil: `--portable`, `--data-dir <ruta>` o `TUNNELBYPASS_DATA_DIR` para controlar dónde se almacenan las configuraciones y registros.
- Rutas de datos predeterminadas:
  - Windows: `C:\TunnelBypass\`
  - Linux: `/usr/local/etc/tunnelbypass/`
- Documentación completa: [`docs/wiki/`](docs/wiki/README.md)
- Guía inicial en árabe: [`docs/wiki/Home.md`](docs/wiki/Home.md#arabic-beginners)

---

<h2>Demostración</h2>

<p align="center">
  <a href="https://www.youtube.com/watch?v=praPwuKj9d4">
    <img src="https://img.youtube.com/vi/praPwuKj9d4/maxresdefault.jpg" alt="TunnelBypass — configuración" width="70%">
  </a>
</p>

---

<h2>Licencia</h2>

<p><a href="LICENSE">MIT</a></p>

---

<p align="center">Hecho con ❤️ en Egipto</p>
