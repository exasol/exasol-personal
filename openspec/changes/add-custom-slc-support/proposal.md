## Why

Official script language containers cover the standard `PYTHON3`, `JAVA`, and `R` runtimes,
but users evaluating Exasol extensibility often need their own container — a Python with
extra packages, or a runtime the catalog does not ship. The documented production way to
install such a container is to place it in BucketFS and activate it with
`ALTER SYSTEM SET SCRIPT_LANGUAGES`, choosing an alias. Local Exasol Personal deployments
have no first-class command for this, so users would have to do it by hand.

## What Changes

- Add `exasol slc custom install`/`update`/`remove` for user-supplied containers: `--file
  <tarball>` or `--url <https-url>`, plus `--alias <NAME>` and `--language <python|java|r>`.
  The top-level `install`/`update`/`remove` stay official-only.
- Materialize a custom container by unpacking it into the default BucketFS bucket and
  activating it with `ALTER SYSTEM SET SCRIPT_LANGUAGES` (read-merge-write, preserving every
  other alias). This activates for new sessions without a restart.
- Persist installed custom SLCs in a separate launcher-state list from official ones,
  identified by content digest, so the start path that re-applies image mounts never sees a
  custom SLC.
- Unpack each version into a content-addressed BucketFS directory so a replace never overwrites
  the active container: the new version is extracted, activated, and recorded before the
  previous directory is removed, and any failure before that leaves the old container usable.
- `exasol slc list` shows custom SLCs alongside official ones; `exasol slc custom
  install`/`update`/`remove` manage them.
- Enforce alias mutual-exclusivity between custom and official SLCs in both directions: a
  custom install is blocked when the alias is owned by an installed official SLC, asks for
  confirmation before overriding a built-in alias, and — mirrored — an official install is
  blocked when a custom SLC already owns one of its aliases. When a custom SLC overrode a
  built-in alias, removing it restores the built-in mapping rather than leaving the alias
  undefined.
- Download for `--url` happens on the host and the container is streamed into the
  deployment's BucketFS; the downloaded copy is removed after unpacking so a large container
  does not linger on the user's disk.
- Validate the container on the host before writing anything into the deployment: verify
  archive integrity, reject entries that would escape the container directory, and require a
  standard SLC layout — so a corrupt or non-SLC archive is rejected before the database's
  BucketFS is touched. After activation, confirm the alias is present in `SCRIPT_LANGUAGES`
  before reporting success.
- Scope to local (darwin/arm64) deployments only, and require a running database (activation
  goes through `ALTER SYSTEM`).

## Capabilities

### Modified Capabilities

- `local-slc-management`: adds installing, updating, listing, and removing user-supplied
  (custom) script language containers, and extends alias-uniqueness enforcement to span
  custom and official SLCs.

## Impact

- `internal/config`: a new `InstalledCustomSLC` state list, separate from `InstalledSLC`.
- `internal/customslc`: pure logic for the activation URI and SCRIPT_LANGUAGES read-merge-write.
- `internal/deploy`: custom install/update/remove/list orchestration, plus the mirror
  alias-collision guard on the official install/update path. Container delivery is a
  backend-specific step: the local backend writes into the VM's BucketFS over SFTP, while a
  cloud deployment (reached through the BucketFS HTTP service) is a separate backend
  implementation.
- `internal/remote`: SFTP-based remote filesystem access, so container files are delivered
  in pure Go without depending on a remote shell or remote archiving tools.
- `cmd/exasol`: an `slc custom install`/`update`/`remove` command group; `slc list` covers both,
  and the top-level `remove` points custom aliases at `slc custom remove`.
- Backward compatible: official SLC behavior and its state are unchanged; the new custom
  state list defaults to empty.
