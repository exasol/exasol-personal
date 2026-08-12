## 1. State and pure logic

- [x] 1.1 Add a custom SLC state list, separate from the official one, recording alias, language, image reference, mount target, staged package name, content digest, source, activation flag, and the displaced built-in URI.
- [x] 1.2 Build the activation URI against the database's built-in bucket and the SCRIPT_LANGUAGES read-merge-write as pure, unit-tested logic.
- [x] 1.3 Derive the image reference, mount directory (`custom-` prefixed), and package name from alias and content digest.
- [x] 1.4 Expose the staging location as a launcher-owned path, so staging does not depend on parsing local runtime state.

## 2. Delivery

- [x] 2.1 Stage a validated container tarball where the local runtime can reach it, writing atomically so a partially staged package can never be imported.
- [x] 2.2 Contribute both a mount and a package reference per custom SLC from the start path, so the runtime imports and mounts it on every start.
- [x] 2.3 Read the runtime's per-image materialization report to learn whether a custom container is available.
- [x] 2.4 Delete a superseded or removed container's staged package.

## 3. Custom install / update / remove

- [x] 3.1 `slc custom install --source --alias --language`: validate, acquire (with digest), stage, record state, then apply through a restart; `--no-restart` records without restarting.
- [x] 3.2 Reconcile pending activation once the database is ready on every start, warning without failing the start.
- [x] 3.3 Refuse to activate an alias whose container the runtime reported as unavailable, and report which container is unavailable and why.
- [x] 3.4 `slc custom update` replaces the container behind a custom alias; identical content and language is a no-op.
- [x] 3.5 `slc remove` deactivates a custom container (restoring a displaced built-in) and deletes the staged package; a running database is required only for an active container, since a never-activated one holds no alias.
- [x] 3.6 Treat an inactive recorded container as retryable rather than as an already-installed no-op.

## 4. Alias mutual-exclusivity (both directions)

- [x] 4.1 Custom install: block reuse of an alias owned by an installed official SLC; confirm before overriding a built-in/official alias that is not installed; confirm before replacing an installed custom SLC.
- [x] 4.2 Official install/update: block when a custom SLC already owns one of the official SLC's aliases.

## 5. CLI surface

- [x] 5.1 Custom install and update live under `slc custom` with `--source`; the top-level `install`/`update` stay official-only, while the top-level `remove` handles both kinds and `slc custom remove` remains as a hidden alias.
- [x] 5.2 Add `--no-restart` and `--auto-approve` to the custom install/update commands and report the apply outcome.
- [x] 5.3 `list` (text and `--json`) covers custom SLCs alongside official ones, distinguished by type and reporting availability.

## 6. Container validation

- [x] 6.1 Validate the archive on the host before staging: verify integrity and require the standard SLC client to be present.
- [x] 6.2 Confirm activation took effect (read back `SCRIPT_LANGUAGES`) before reporting success.

## 7. Tests and validation

- [x] 7.1 Unit tests for the activation URI, the derived image/mount/package names, the SCRIPT_LANGUAGES merge, alias/source validation, and the collision rules in both directions.
- [x] 7.2 Unit test that the start path contributes a mount and a package reference for each custom SLC.
- [x] 7.3 Unit tests for the activation reconcile: deferred install, unavailable container, and no-op when nothing is pending.
- [x] 7.4 Unit tests for archive validation and package staging.
- [x] 7.5 End-to-end coverage of a custom install running a UDF, and of `--no-restart` activating on the next start.
- [x] 7.6 Formatting, focused tests, full-repo tests, and OpenSpec validation for this change.
