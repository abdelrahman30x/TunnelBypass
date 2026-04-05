# Installation

[← Handbook](README.md)

## From source

Requires **Go** version per `go.mod`. From repository root:

```bash
go build -trimpath -ldflags "-s -w" -o tunnelbypass ./cmd
./tunnelbypass --version
```

Optional: embed a version string via `-ldflags` on the main package (see root README).

## Install scripts (releases)

Prebuilt binaries: use the install scripts referenced in the root **README** (`scripts/install.sh`, `scripts/install.ps1`). They resolve the latest release asset for your OS/arch and optionally place the binary under `INSTALL_PREFIX`.

Pinned version: set `INSTALL_VERSION` to a tag when you need reproducible installs.

## Docker

Build a local image and run with a **data volume** mapped so configs survive restarts. The environment variable **`TUNNELBYPASS_DATA_DIR`** (e.g. `/data` in examples) anchors configs and logs inside the container.

See `docker-compose.yml` and `docker/env.example`. Treat **service installation inside the container** as optional—many operators prefer `run portable` or foreground `run` for server workloads in Docker.

## Data directories (mental model)

Without overrides, defaults are per-user on desktop OSes and documented system paths for “system” layouts. **`--data-dir`**, **`run portable`**, and **`TUNNELBYPASS_DATA_DIR`** exist so you can:

- Keep everything under a single folder (backups, migrations).
- Run without elevation when the transport allows it.
- Separate multiple instances by directory.

Exact tables: root **README** → *Data dirs*.
