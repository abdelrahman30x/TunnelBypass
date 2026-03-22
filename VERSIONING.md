# Versioning

## Semver

Versions are **MAJOR.MINOR.PATCH** (example: `v1.2.0`):

| Bump | When |
|------|------|
| **MAJOR** | Breaking CLI behavior, config path breaks, or removals operators rely on |
| **MINOR** | New commands, transports, or features; backward compatible |
| **PATCH** | Bug fixes, docs, internal refactors with no intended behavior change |

Pre-releases can use `-alpha.N` or `-rc.N` tags if needed.

## Source vs Git tag

The default in [`cmd/version.go`](cmd/version.go) (`var Version`) should match the **latest released tag** (e.g. `v1.2.0`). Bump it in the same commit as the tag, or immediately after tagging. Release builds may instead inject the tag with `-ldflags "-X main.Version=..."`. The internal default in [`internal/cli/app.go`](internal/cli/app.go) should stay in sync for `go test` / tools that call `Main` without `SetVersion`. The binary entrypoint is [`cmd/main.go`](cmd/main.go).

## Releases on GitHub

1. Update `CHANGELOG.md` (move items from `[Unreleased]` into a dated section).
2. Set `Version` in `cmd/version.go` (and the fallback `version` in `internal/cli/app.go` if you rely on tests without `SetVersion`).
3. Tag: `git tag -a v1.2.1 -m "v1.2.1"`
4. Push the tag; attach binaries (optional) built with `make build` or `go build -o tunnelbypass ./cmd`.

After publishing, add compare links at the bottom of `CHANGELOG.md` using your real `github.com/org/repo` path.
