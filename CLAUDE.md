# CLAUDE.md

Project-specific notes for working in this repository.

## Gotchas

### `exasol install` reuses an already-initialized deployment dir

`exasol install` does **not** re-extract the binary's `//go:embed` installation
assets if the target deployment directory is already initialized. It logs
*"deployment directory is already initialized with the requested presets"* and
runs the preset files already extracted into
`~/.exasol/personal/deployments/<name>/installation/`.

Consequence: after editing an embedded asset (e.g.
`assets/installation/ubuntu/files/opt/exasol_launcher/scripts/installAiLab.sh`)
and rebuilding the CLI, a plain `install` into an existing deployment dir still
runs the **old** script. To validate changes to embedded installation assets
end-to-end on a real deployment, re-initialize first:

```bash
exasol destroy --remove   # or `exasol remove` if resources are already gone
exasol install <preset>   # now the updated assets are extracted
```

(Confirmed empirically: a fresh `--with-ai-lab` deploy ran a stale
`installAiLab.sh` because the default deployment dir was already initialized.)

## Building

Preferred: `task build` (Taskfile `build` target) — produces `bin/exasol`.

If `task` is not installed, the equivalent is:

```bash
go run ./tools/localrunner placeholder \
  -target assets/localruntimebin/generated/darwin/arm64/mac-runner-aarch64  # go:embed placeholder
go generate ./...
version=$(git describe --tags --abbrev=0)
CGO_ENABLED=0 go build -o bin/exasol -trimpath -gcflags=all=-l \
  -ldflags "-s -w -X main.CurrentLauncherVersion=${version:1}" ./cmd/exasol
```
