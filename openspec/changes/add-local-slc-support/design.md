# Design — Local SLC support (Workflow 1: official, local, image mount)

## Context

Local deployments run nano inside a QEMU VM managed by an embedded runner
(`exasol-local-vm`). The launcher never runs Podman directly; the runner boots the VM and
`init-db.sh` (from the runner's embedded assets) issues the single `podman run` that
starts the nano database container. Script language aliases are read by the database from
each SLC's `build_info/language_definitions.json`; the nano `admini` component scans
`/exa/slc` on start and auto-registers builtin aliases (verified in code:
`db/COSMock/src/admini`). No `ALTER SYSTEM` is required for official SLCs.

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
  4. launcher stops then starts the deployment (via the runner)
  5. runner writes vm-shared/slc.json from --slc start flags
  6. init-db.sh reads slc.json, force-recreates the container with --mount type=image
     (podman pulls the image inside the VM), mounting it at /exa/slc/<target>
  7. nano admini scans /exa/slc -> registers PYTHON3; launcher waits-for-ready and verifies
```

Image mounts are `podman run` arguments and are not persisted into `/exa`; the launcher
therefore re-passes the full installed set on every start (step 4-5), reconstructed from
launcher state — analogous to how version-check settings are re-passed today.

## Cross-repo contract (exasol-local-vm)

The runner change is small and additive. Backward compatibility rule: when no SLCs are
requested, behavior is byte-for-byte identical to today.

**1. Runner `start` flag (mac runner, `launcher/mac/main.go`).**
Add a repeatable `--slc` flag; each value encodes one mount as `<image>=<target>` (image
references contain no `=`; targets are absolute paths with no `=`, so `=` is an
unambiguous separator — split on the first `=`). The launcher passes one `--slc` per
installed SLC. Mirror `writeVersionCheckRuntimeConfig`: add `writeSlcRuntimeConfig` that
writes the shared file.

**2. `vm-shared/slc.json`** (written by the runner, read by `init-db.sh`):
```json
{ "slc": [ { "image": "docker.io/exasol/script-language-container:standard-EXASOL-all-python-3.12-release_arm64_GM7DI5...ISZQ",
             "target": "/exa/slc/python312" } ] }
```
Absent or `{"slc":[]}` means "no SLCs" and must be treated identically to today.

**3. `init-db.sh` — read the list.** After the existing config parsing, add:
```sh
SLC_CONFIG_FILE="$EXASOL_VM_HOST_SHARED_DIR/slc.json"
build_slc_mount_args() {   # prints repeated: --mount type=image,source=<img>,destination=<dst>
  [ -f "$SLC_CONFIG_FILE" ] || return 0
  count=$(jq -r '.slc // [] | length' "$SLC_CONFIG_FILE") || return 1
  i=0
  while [ "$i" -lt "$count" ]; do
    img=$(jq -er ".slc[$i].image"  "$SLC_CONFIG_FILE") || return 1
    dst=$(jq -er ".slc[$i].target" "$SLC_CONFIG_FILE") || return 1
    printf '%s\n' "--mount" "type=image,source=$img,destination=$dst"
    i=$((i + 1))
  done
}
```
Accumulate into positional parameters (the file already uses the `set --` idiom) so values
are never re-split by the shell.

**4. `init-db.sh` — harden the recreate.** Replace the current soft removal
(`podman rm … || true`, lines ~350-353) with a forced, verified removal, and add
`--replace` to `podman run`:
```sh
if podman container exists "$DB_CONTAINER_NAME"; then
  log_msg "Removing existing container $DB_CONTAINER_NAME to recreate it fresh"
  podman rm -f "$DB_CONTAINER_NAME" || { log_msg "Error: failed to remove $DB_CONTAINER_NAME"; log_diagnostics; exit 1; }
fi
if podman container exists "$DB_CONTAINER_NAME"; then
  log_msg "Error: $DB_CONTAINER_NAME still present after removal; aborting"; exit 1
fi
# podman run -d --replace ... <existing args> $SLC_MOUNT_ARGS "$@"
```
Rationale: a stale container (from an unclean VM kill) otherwise makes `podman run --name`
fail with a name conflict, so the new SLC-mounted container never starts. `rm -f` handles
any container state; the post-removal check and `exit 1` replace the failure-swallowing
`|| true`; `--replace` is defense-in-depth against a race.

**5. Re-embed the runner** into exasol-personal via `tools/localrunner` after the change.

## exasol-personal design

- **Catalog** (`assets/resources/slc-catalog.yaml`, already added): embedded like
  `resources.yaml`. Loader resolves an alias to `{image ref, target dir, declared aliases}`
  using `default_version` and the flavor's `aliases` list.
- **Alias resolution**: case-insensitive match against declared aliases
  (`PYTHON3, PYTHON312, JAVA, JAVA17, R, R44`). Unknown alias → error listing valid aliases.
- **Target directory**: derived deterministically from the flavor (e.g.
  `/exa/slc/python312`); free-form as far as the DB is concerned (aliases come from the
  image), must be unique per installed SLC, and MUST NOT start with `current-` (that prefix
  is reserved by the runner's managed `current-java` anchor and is skipped by the scan).
- **State**: the installed SLC set is persisted in launcher state in the deployment
  directory and is the source of truth re-applied on every start.
- **Collision rule (full alias-disjointness)**: the set of mounted SLCs MUST be disjoint
  across *all* declared aliases — versioned and unversioned — because the DB throws at
  engine init on *any* duplicate alias (`SlcConfig::createBuiltinNameMap`), not only on a
  duplicated unversioned one. Before an install the launcher compares the candidate SLC's
  full alias set against that of every already-installed SLC: a newer version of an
  already-installed flavor **replaces** the incumbent; any other overlap is **rejected**
  with a clear message naming the conflicting alias and the SLC that already owns it. Alias
  sets are read per installed `(version, flavor)` from the catalog, because the owner of an
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
  confirmation first. `--yes` skips the prompt (required for automation) and `--no-restart`
  records the change to apply on the next start without restarting now. The prompt is shown
  only when a restart will actually occur (running database) and only after validation
  (unknown alias / collision fail before any prompt); a non-interactive session without
  `--yes`/`--no-restart` is refused rather than silently restarting. A guard against
  restarting during a backup/critical operation was considered but deferred: Personal-local
  has no reliable "operation in progress" signal to detect today, so confirmation is the v1
  safeguard.
- **Backend scoping**: only the local (darwin/arm64) backend supports these commands; other
  backends return a clear "unsupported" error.

## Error handling / edge cases

- Unknown alias → error with the list of valid aliases; no state change, no restart.
- Non-local backend → unsupported error; no state change.
- Alias collision → rejected with the conflicting alias/flavor named; no state change.
- Image pull / container recreate fails on start → `init-db.sh` exits non-zero; the launcher
  reports the failure and that the SLC is configured-but-not-active (never a false success).
- Re-install of an already-installed alias → no-op (same version) or replace (different
  version), idempotent either way.
- Removing a not-installed alias → clear message, no restart.

## Storage hygiene

On replace or removal, the recreated container drops the old mount but the old SLC image
would otherwise remain in the VM's Podman store, growing it unboundedly across install
cycles (a concern the broader SLC design explicitly called out). `init-db.sh`
(`prune_unreferenced_slc_images`) reclaims it: after the desired images are pulled and
before `podman run` — the point at which the outgoing DB container is already removed, so
old images are unreferenced — it removes any `exasol/script-language-container` image whose
reference is not listed in the current `slc.json`. The prune is deliberately narrow and
safe:

- **Scoped to the SLC repository.** Only images whose reference contains
  `exasol/script-language-container` are eligible; the DB image and any unrelated images are
  never considered.
- **Authoritative-set only.** It runs only when `slc.json` exists. An absent file means
  "SLC-unaware" (an older launcher, or today's default), so the store is left untouched
  rather than pruned against an unknown desired set.
- **Best-effort, never fatal.** A removal that fails (image still in use, or shared layers)
  is logged and skipped; pruning can never abort database startup.

Dangling (`<none>`) SLC layers left by an overwritten tag are out of scope — each SLC
version carries a unique content-hash tag, so replaced/removed images retain their
`repo:tag` and are matched by reference.

## Risks

- Image mounts are non-persistent → correctness depends on the launcher re-passing the full
  installed set on every start. State is the single source of truth.
- The runner change requires rebuilding and re-embedding the runner binary; the darwin/arm64
  runner is `go:embed`-ed and staged via `tools/localrunner`.
