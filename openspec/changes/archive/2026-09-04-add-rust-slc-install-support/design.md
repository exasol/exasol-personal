## Context

`exasol slc install <alias>` resolves an alias against the official SLC catalog
(`assets/resources/slc-catalog.yaml`), whose entries are pinned by version and sha256.
`exasol slc custom install` installs a user-supplied container from a tarball or an `https` URL
under a chosen alias, validated by archive shape rather than by checksum. Since
`support-custom-slc-language-identifiers`, `--language rust` is already accepted, so a Rust
container can be installed today — but only if the user resolves the release asset themselves.

The Rust container is published as GitHub releases of `exasol-labs/language-container-rs`, with
architecture-specific assets whose file names contain the version (`lc-rust-<version>.tar.gz` for
x86_64, `lc-rust-<version>-aarch64.tar.gz` for aarch64). It is a prerequisite for the Rust UDFs of
`lakehouse-engine-rs`.

## Goals / Non-Goals

**Goals:**

- Make the Rust container installable with one command that needs no release lookup by the user.
- Keep the existing `slc install <alias>` / `slc update <alias>` CLI shape and flag set.
- Reuse the custom-SLC path so state, activation, restart, no-op, list, and remove behavior is
  inherited rather than re-implemented.

**Non-Goals:**

- Pin or verify a specific Rust container build; that is what `slc custom install --source` is for.
- Add the Rust container to the official SLC catalog.
- Support architectures for which the project publishes no release asset.
- Detect or protect a `RUST` alias that was set up outside the launcher.

## Decisions

### `rust` is an alias value, not a new subcommand

`slc install rust` and `slc update rust` special-case the alias `rust` before the official catalog
lookup and dispatch it to the Rust install path. The alias keeps the same flags as every other
alias (`--auto-approve`, `--no-restart`, `--deployment`/`--deployment-dir`, `--json`, `-v`).

An earlier draft added `exasol slc rust install` as its own subcommand and was rejected: it breaks
the established `exasol slc install <alias>` structure, and it would need its own copies of the
flags, help, and deployment resolution that the alias path already has.

### Resolve `latest` through the GitHub releases API

The launcher reads the tag of
`https://api.github.com/repos/exasol-labs/language-container-rs/releases/latest` and then builds
the asset download URL from that tag.

GitHub's fixed-name shortcut `/releases/latest/download/<name>` was rejected because it requires a
version-independent asset name, and these asset names embed the version. Pinning a version inside
the launcher was also rejected: it would tie every Rust container release to a launcher release,
which is exactly the coupling this change removes.

### Select the asset by host architecture, before any network call

`amd64` maps to `lc-rust-<version>.tar.gz` and `arm64` to `lc-rust-<version>-aarch64.tar.gz`. Any
other architecture fails with a clear error and no network call, so an unsupported platform never
waits on `api.github.com`.

### Install as a custom SLC under the fixed alias `RUST`

The Rust path delegates to the existing custom-SLC install with `--alias RUST --language rust`.
This is why `slc remove rust` and `slc list` need no new code: once installed, `RUST` is an
ordinary custom-SLC entry in launcher state, and the generic alias-state machinery already handles
listing, availability, removal, deactivation, package cleanup, and pending activation. Restart
confirmation, `--auto-approve`, `--no-restart`, and the identical-content no-op are inherited from
the same path, so `slc update rust` is a re-resolution followed by the same install.

### No `--source` flag on `slc install rust`

`slc install <alias>` has no `--source` flag, and adding one for a single alias would make the flag
set alias-dependent. Users who need a specific build use the pre-existing
`exasol slc custom install --alias RUST --language rust --source <path-or-url>`, which already
works and gives them a pinned source they control.

### Accept the custom-SLC trust bar, do not claim catalog parity

The download is protected by TLS, the existing `https`-only redirect guard, and post-download
archive-shape validation (the tarball must contain an executable `exaudf/exaudfclient`). There is
no checksum or signature check against a known-good value, because `latest` is a moving target and
cannot be hash-pinned. This is the same trust bar the already-accepted
`slc custom install --source <https-url>` has, and deliberately weaker than the official catalog's
version- and sha256-pinned artifacts.

## Risks / Trade-offs

- [An unpinned `latest` download can change content between two installs] → Same, already accepted
  bar as `slc custom install --source <https-url>`; users who need reproducibility pin the source
  through `slc custom install`.
- [A `RUST` alias hand-installed outside the launcher — for example by editing `SCRIPT_LANGUAGES`
  through SQL — is silently taken over on activation] → Accepted. The official/custom
  alias-collision guard only fires for aliases the official catalog declares, and a hand-edit is
  neither official nor tracked in launcher state, so there is nothing to detect. This change does
  not fix that.
- [`api.github.com` is unreachable or rate-limits the request] → Fail before staging anything, with
  an error that names the release URL, and document `slc custom install --source` as the offline or
  pinned alternative.
- [Upstream changes its release asset naming] → The naming pattern lives in one place, and the
  install fails on a missing asset rather than installing something unexpected.
