## Why

Official script language containers cover the standard `PYTHON3`, `JAVA`, and `R` runtimes,
but users evaluating Exasol extensibility often need their own container — a Python with
extra packages, or a runtime the catalog does not ship. Local Exasol Personal deployments
have no first-class command for this, so users would have to do it by hand.

A custom container should reach the database the same way an official one does: as a Podman
image mounted into the database container. The only two things a user-supplied container cannot
borrow from the official path are delivery (it exists as a tarball on the user's disk, not in a
registry) and alias registration (containers built from the public flavor templates carry no
language-definition metadata, so the database cannot discover them by itself, and that
discovery only fills the fixed built-in alias slots in any case). Everything else — mounting,
restart semantics, image reclamation — is shared with the official path.

## What Changes

- Add `exasol slc custom install`/`update` for user-supplied containers, removed through the
  common `exasol slc remove <alias>`:
  `--source <tarball-or-https-url>`, plus `--alias <NAME>` and `--language <python|java|r>`.
  The top-level `install`/`update`/`remove` stay official-only.
- Deliver a custom container by staging its tarball where the local runtime can reach it; the
  local runtime imports it as a Podman image and mounts it into the database container at start,
  the same way an official container is pulled and mounted.
- Activate a custom container with a single `ALTER SYSTEM SET SCRIPT_LANGUAGES`
  (read-merge-write, preserving every other alias) using a URI that addresses the mounted
  directory through the database's built-in BucketFS bucket.
- Apply through a restart, like the official path, because a Podman image mount is a
  container-run argument and does not survive container recreation: `--auto-approve` skips the
  confirmation and `--no-restart` records the container so it is mounted and activated on the
  next start.
- Persist installed custom SLCs in a separate launcher-state list carrying the image
  reference, mount target, staged package name, content digest, and whether activation has
  been applied.
- Derive the image reference, mount directory, and package name from the alias and content
  digest, with a `custom-` prefixed mount directory so custom containers can never collide
  with official mount directories or the database's own directories.
- Reconcile pending activation on every start: a container recorded but not yet activated is
  activated once the database is ready, which is what makes `--no-restart` and an interrupted
  install converge without user action.
- Keep the database startable when a custom container cannot be materialized (the staged
  package is missing or not importable): the local runtime reports the per-container outcome,
  the launcher skips activation and reports which container is unavailable, and the deployment
  still comes up.
- `exasol slc list` shows custom SLCs alongside official ones with their availability.
- Enforce alias mutual-exclusivity between custom and official SLCs in both directions, and
  restore a displaced built-in alias when a custom SLC that overrode it is removed.
- Validate the tarball on the host before staging — archive integrity plus the standard SLC
  client — so a wrong or corrupt file is rejected before a restart rather than after one.
- Scope to local deployments only.

## Capabilities

### Modified Capabilities

- `local-slc-management`: adds installing, updating, listing, and removing user-supplied
  (custom) script language containers; extends alias-uniqueness enforcement to span custom and
  official SLCs; extends restart, verification, and image-reclamation behavior to custom
  containers.

## Impact

- `internal/config`: a new `InstalledCustomSLC` state list, separate from `InstalledSLC`.
- `internal/customslc`: pure logic for the activation URI, the SCRIPT_LANGUAGES
  read-merge-write, and container validation.
- `internal/localruntime`: the launcher-owned location where a container package is staged for
  the local runtime to import.
- `internal/deploy`: custom install/update/remove/list orchestration, package staging, the
  start-time activation reconcile, and the mirror alias-collision guard on the official path.
  The start path contributes both a mount and a package reference for each custom SLC.
- `cmd/exasol`: an `slc custom install`/`update` command group; `slc list` and the top-level
  `slc remove` cover both kinds, with `slc custom remove` kept as a hidden alias.
- Backward compatible: official SLC behavior and its state are unchanged; the new custom
  state list defaults to empty.
- Requires a local runtime that can import a staged container package; where that capability is
  absent, custom containers cannot be materialized.
