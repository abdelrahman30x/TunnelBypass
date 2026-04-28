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

## Data directories (mental model)

Without overrides, defaults are per-user on desktop OSes and documented system paths for “system” layouts. **`--data-dir`**, **`run portable`**, and **`TUNNELBYPASS_DATA_DIR`** exist so you can:

- Keep everything under a single folder (backups, migrations).
- Run without elevation when the transport allows it.
- Separate multiple instances by directory.

Exact tables: root **README** → *Data dirs*.
