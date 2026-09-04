## Why

Rust UDFs — such as those shipped by `lakehouse-engine-rs` — need the Rust script language
container from `exasol-labs/language-container-rs`. Today a user has to look up the right release
asset for their CPU architecture themselves and install it through
`exasol slc custom install --source <url>`.

## What Changes

- Accept `rust` as an alias of the existing `exasol slc install <alias>` and
  `exasol slc update <alias>` commands, with the same flags as every other alias.
- Resolve the latest `exasol-labs/language-container-rs` release and pick the asset matching the
  host CPU architecture, then install it under the fixed alias `RUST`.
- Read the release tag from the GitHub releases API, because the asset file names embed the
  version and GitHub's fixed-name `releases/latest/download` shortcut therefore cannot be used.
- Document `exasol slc custom install --alias RUST --language rust --source <path-or-url>` as the
  way to install a specific Rust container instead of the current latest one.
- `exasol slc remove rust` and `exasol slc list` are unchanged; they already handle `RUST` as a
  custom-SLC alias in launcher state.

## Capabilities

### New Capabilities

- None.

### Modified Capabilities

- `local-slc-management`: `slc install <alias>` and `slc update <alias>` resolve the reserved
  alias `rust` from the latest Rust language container release instead of the official SLC catalog.

## Impact

- Adds `api.github.com` as an external dependency of `slc install rust` and `slc update rust`, in
  addition to the existing release download from `github.com`.
- Reuses the existing custom-SLC staging, activation, state, restart, and no-op behavior; no new
  database or SLC protocol behavior.
- `latest` is by definition unpinned, so the download carries the same trust bar as
  `slc custom install --source <https-url>` — TLS, an `https`-only redirect guard, and
  archive-shape validation, but no checksum or signature verification. This is deliberately weaker
  than the version- and sha256-pinned official SLC catalog.
