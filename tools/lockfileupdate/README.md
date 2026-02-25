# Lockfile Update Tool

This tool regenerates OpenTofu/Terraform provider lockfiles (`.terraform.lock.hcl`) for the infrastructure presets under `assets/infrastructure/`.

It exists to keep lockfiles **valid across platforms** (Linux/macOS/Windows) while keeping the `assets/` directories **free of temporary OpenTofu artifacts** such as `.terraform/`.

## Why this exists

Provider lockfiles contain checksums for the *platform-specific* provider packages.
If a lockfile is generated on Linux, it may only contain Linux provider hashes.
Running `tofu init -lockfile=readonly` on Windows will then fail because the Windows provider package hash is missing.

This tool updates the lockfile by running `tofu providers lock` with a configured list of platforms so the committed lockfile works on all supported developer/CI platforms.

## What it does (high level)

For each infrastructure preset directory under `assets/infrastructure/*`:

1. Detect whether the preset actually uses OpenTofu.
   - It must have an `infrastructure.yaml` with a `tofu:` section, **and**
   - it must contain at least one `.tf` file.
   Presets that don’t use OpenTofu are skipped.
2. Copy the preset directory into a temporary working directory.
3. Write the embedded OpenTofu binary into the temporary directory.
4. Run `tofu providers lock` for multiple `-platform=...` values.
5. Copy the generated `.terraform.lock.hcl` back into the original preset directory in `assets/`.
6. Remove the temporary directory.

The result is a clean `assets/` tree with updated lockfiles and no leftover OpenTofu working directories.

## How to use

### Recommended (via Task)

From the repository root:

- Run `task generate` (downloads embedded OpenTofu binaries)
- Run `task tofu-lock-update`

See the development guide for the canonical workflow context:
- `doc/development.md`

### Direct invocation

From the repository root:

- `go run ./tools/lockfileupdate/main.go`

Optional flags:

- `-infra-assets-dir=...` to point at a different infrastructure assets root (default: `./assets/infrastructure`).
- `-preset <name>` (repeatable) to update only specific preset directories.
- `-platform <os_arch>` (repeatable) to override the default platform list.
- `-v` for verbose output.

Example (update only one preset):

- `go run ./tools/lockfileupdate/main.go -preset aws -v`

## Notes / assumptions

- The tool uses the **embedded** OpenTofu binary from the repository (see `assets/tofubin`). If those binaries are placeholders, run `task generate` first.
- The tool intentionally avoids writing state files; it only regenerates lockfiles.
- Platform list defaults are conservative; adjust them if the project’s supported platforms change.
