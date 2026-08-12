## 1. Catalog and alias resolution

- [x] 1.1 Embed and load `assets/resources/slc-catalog.yaml` (mirror the `resources.yaml` embed pattern).
- [x] 1.2 Resolve an alias (case-insensitive) via `default_version` and the flavor `aliases` list to `{image ref, target dir, declared aliases}`.
- [x] 1.3 Return a clear error for an unknown alias, listing the valid aliases.

## 2. Installed-SLC state and collisions

- [x] 2.1 Persist the installed SLC set in launcher state in the deployment directory.
- [x] 2.2 Reject an install whose unversioned alias collides with an already-installed SLC; treat a same-flavor version change as a replace. (Generalized to full alias-disjointness.)

## 3. CLI (`cmd/exasol`)

- [x] 3.1 `exasol slc install <alias>` — no-op (no state change, no restart) when the exact resolved image is already installed; shared image-comparison helper with `update`.
- [x] 3.2 `exasol slc list` (with `--json`).
- [x] 3.3 `exasol slc remove <alias>`.
- [x] 3.4 `exasol slc update <alias>` — re-resolve against the catalog; no-op if the resolved image is unchanged, otherwise replace + restart. Digest/image comparison, not version ordering; no downgrade guard (rollback is out of scope).
- [x] 3.5 Guard to local (darwin/arm64) backend only; clear "unsupported" message otherwise.
- [x] 3.6 `exasol slc list` degrades gracefully on an unsupported architecture: no error, a "none available" text message + `[]` JSON, exit 0 (unlike install/update/remove, which fail explicitly).
- [x] 3.7 install/update/remove refuse cleanly on an initialized-but-not-deployed deployment ("deployment is not present; run `exasol deploy` first"), recording no state.

## 4. Local-runtime wiring (`internal/deploy`, `internal/localruntime`)

- [x] 4.1 Build runner `--slc <image>=<target>` start args from launcher state (mirror `localRunnerVersionCheckArgs`).
- [x] 4.2 Install/update/remove = update state → genuine stop → start.
- [x] 4.3 After restart, wait for readiness (readiness == applied: the runner start fails and the DB never becomes ready if an image cannot be pulled or two SLCs collide); report a clear error (never a false success) if not.
- [x] 4.4 Warn + confirm before restarting a running database; `--auto-approve` skips the prompt, `--no-restart` defers activation to the next start, non-interactive without either is refused. Confirmation happens after validation and only when a restart will actually occur.

## 5. Runner interface (external dependency)

- [x] 5.1 Add a repeatable `--slc <image>=<target>` start flag to the runner.
- [x] 5.2 Mount each requested image into the database container at its target path.
- [x] 5.3 Harden container recreation so a stale container from an unclean shutdown cannot block startup.
- [x] 5.4 Backward compatibility: an empty request set produces the exact current behavior.
- [ ] 5.5 Rebuild and re-embed the runner into exasol-personal via `tools/localrunner` (requires a macOS build host: the runner depends on the darwin-only `vz` framework).
- [x] 5.6 Cover the runner interface with tests: mount present, mount absent, stale-container cleanup, unreferenced-image pruning, and no-prune when SLC-unaware.

## 6. Storage hygiene

- [x] 6.1 Reclaim unreferenced SLC images on replace and remove. Handled by the runner during recreation: scoped to the SLC image repository only (never the DB or unrelated images), skipped when SLC-unaware, and best-effort (a failed removal is logged and skipped, never fatal).

## 7. Tests and validation

- [x] 7.1 Unit tests for catalog load and alias resolution (including case-insensitivity and unknown alias).
- [x] 7.2 Unit tests for the collision rule (reject vs replace, including versioned-alias overlap).
- [x] 7.3 Command wiring verified (registration + flags + help); deploy-side helpers unit-tested (`upsertInstalledSLC`, `findInstalledSLC`, `localRunnerSlcArgs`).
- [x] 7.4 Local-runtime start-arg construction covered via `localRunnerSlcArgs` state round-trip test.
- [x] 7.5 Run formatting, focused tests, and OpenSpec validation for this change.
