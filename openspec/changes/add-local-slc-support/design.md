# Design — Local SLC support (Workflow 1: official, local, image mount)

## Context

Local deployments run the database inside a macOS Virtualization.framework VM managed by an
embedded runner. The launcher never manages containers directly; it drives the runner, which
boots the VM and starts the database container. The database auto-registers a script
language container's aliases by scanning the `/exa/slc` mount directory on start, so no
`ALTER SYSTEM` is required for official SLCs.

This change adds official SLC installation for local deployments only. It is deliberately
one lane of the broader SLC design (custom SLCs, cloud, and the tarball + `ALTER SYSTEM`
path are out of scope here).

## Goals

- `exasol slc install <alias>` / `list` / `update <alias>` / `remove <alias>` for official
  SLCs, local only.
- Activation with no manual SQL (rely on the builtin `/exa/slc` scan).
- Robust database-container recreation (an install must never leave the DB unable to start).

## Non-Goals

- Custom (user-built) SLCs, and the tarball-unpack + `ALTER SYSTEM` path.
- Cloud / tofu backends; amd64 / Windows / Linux hosts (local is darwin/arm64 only today).
- Explicit version pinning UX (`alias@version`); the alias always resolves to the
  catalog's `default_version`. Retained older versions exist only for future rollback.
- Simultaneous multiple versions of the same flavor (they share aliases and cannot coexist).
- Consuming a release manifest (none exists yet); the pinned static catalog is the interim.

## Install flow

```
exasol slc install python3
  1. launcher resolves alias -> {image, target} from slc-catalog.yaml
  2. launcher collision-check: installed set must stay alias-disjoint
  3. launcher records the SLC in launcher state
  4. launcher stops then starts the deployment, passing the installed set as --slc flags
  5. the runner mounts each requested image at /exa/slc/<target> in the database container
  6. the database scans /exa/slc, registers the aliases; launcher waits-for-ready and verifies
```

Image mounts are container-run arguments and are not persisted; the launcher therefore
re-passes the full installed set on every start (step 4), reconstructed from launcher state
— analogous to how version-check settings are re-passed today.

## Runner interface

The launcher depends on a small, additive runner interface. **Backward compatibility
rule:** when no SLCs are requested, behavior is identical to today.

**1. `start` flag.** The runner accepts a repeatable `--slc` flag; each value encodes one
mount as `<image>=<target>`. Image references contain no `=` and targets are absolute paths
with no `=`, so the first `=` is an unambiguous separator. The launcher passes one `--slc`
per installed SLC.

**2. Mount semantics.** The runner mounts each requested image into the database container
at its target path. An empty request set is treated identically to today (no SLC mounts).

**3. Runner version.** The embedded runner in `assets/resources/resources.yaml` must be
bumped to a release that supports `--slc` before this feature is enabled; an older runner
rejects the unknown flag and fails to start.

## exasol-personal design

- **Catalog** (`assets/resources/slc-catalog.yaml`, already added): embedded like
  `resources.yaml`. Loader resolves an alias to `{image ref, target dir, declared aliases}`
  using `default_version` and the flavor's `aliases` list.
- **Alias resolution**: case-insensitive match against declared aliases
  (`PYTHON3, PYTHON312, JAVA, JAVA17, R, R44`). Unknown alias → error listing valid aliases.
- **Target directory**: derived deterministically from the flavor (e.g.
  `/exa/slc/python312`); free-form as far as the DB is concerned (aliases come from the
  image), must be unique per installed SLC, and MUST NOT start with `current-` (that prefix
  is reserved by the runner and is skipped by the scan).
- **State**: the installed SLC set is persisted in launcher state in the deployment
  directory and is the source of truth re-applied on every start.
- **Collision rule (full alias-disjointness)**: the set of mounted SLCs MUST be disjoint
  across *all* declared aliases — versioned and unversioned — because the DB throws at
  engine init on *any* duplicate alias, not only on a duplicated unversioned one. Before an
  install the launcher compares the candidate SLC's full alias set against that of every
  already-installed SLC: a newer version of an already-installed flavor **replaces** the
  incumbent; any other overlap is **rejected** with a clear message naming the conflicting
  alias and the SLC that already owns it. Alias sets are read per installed
  `(version, flavor)` from the catalog, because the owner of an
  unversioned alias can shift between releases (e.g. `PYTHON3` moving from python-3.12 to a
  newer flavor), so a flavor's alias set is not release-invariant.
- **Update semantics (digest diff, no version ordering)**: `update <alias>` re-resolves the
  alias against the catalog and compares the resolved **image reference** (content-addressed,
  so an identical reference means identical content) with the installed one. Unchanged → a
  no-op with no restart. Changed → replace the installed entry and restart, exactly like an
  install. Because a version bump can move the unversioned alias to a new flavor (e.g.
  `python-3.12` → `python-3.13`), the replaced entry is identified by the alias match and the
  collision check runs against the *other* installed SLCs so the update never self-collides.
  Rollback/downgrade is out of scope, so update deliberately has no
  "older version" guard — it installs whatever the catalog resolves to now.
- **Restart + verify**: install/update/remove performs a genuine stop→start (a plain start
  no-ops if the container is already running, so the mount would not apply), waits for
  readiness, and verifies the change took effect before reporting success.
- **Restart confirmation**: because activation restarts a running database (dropping open
  connections and aborting running statements), install/update/remove warn and require
  confirmation first. `--auto-approve` skips the prompt (required for automation) and `--no-restart`
  records the change to apply on the next start without restarting now. The prompt is shown
  only when a restart will actually occur (running database) and only after validation
  (unknown alias / collision fail before any prompt); a non-interactive session without
  `--auto-approve`/`--no-restart` is refused rather than silently restarting. A guard against
  restarting during a backup/critical operation was considered but deferred: Personal-local
  has no reliable "operation in progress" signal to detect today, so confirmation is the v1
  safeguard.
- **Backend scoping**: only the local (darwin/arm64) backend supports these commands; other
  backends return a clear "unsupported" error.

## Error handling / edge cases

- Unknown alias → error with the list of valid aliases; no state change, no restart.
- Non-local backend → unsupported error; no state change.
- Alias collision → rejected with the conflicting alias/flavor named; no state change.
- Image pull / container recreate fails on start → the runner start fails; the launcher
  reports the failure and that the SLC is configured-but-not-active (never a false success).
- Re-install of an already-installed alias → no-op (same version) or replace (different
  version), idempotent either way.
- Removing a not-installed alias → clear message, no restart.

## Storage hygiene

On replace or removal the old SLC image would otherwise accumulate in the VM's image store,
growing it unboundedly across install cycles. Reclaiming those unreferenced images is the
runner's responsibility during recreation; the launcher's contribution is to pass the
authoritative installed set on every start, so the runner always knows which SLC images are
still wanted and which are safe to drop. Each SLC version carries a unique content-hash tag,
so replaced or removed images are matched by their exact reference.

## Risks

- Image mounts are non-persistent → correctness depends on the launcher re-passing the full
  installed set on every start. State is the single source of truth.
- The runner change requires rebuilding and re-embedding the runner binary; the darwin/arm64
  runner is `go:embed`-ed and staged via `tools/localrunner`.
