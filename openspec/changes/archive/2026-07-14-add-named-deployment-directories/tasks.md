## 1. Deployment Directory Layout and Name Validation

- [x] 1.1 Add `NamedDeploymentDirPath(name string) (string, error)` to `internal/config/deployment_dir.go`, alongside `DefaultDeploymentDirPath`, joining the launcher's deployments directory with `name`.
- [x] 1.2 Add a `pflag.Value` for `--deployment`/`-d` (mirroring `AbsDirValue` in `cmd/exasol/absdirvar.go`) that validates against `^[A-Za-z0-9_-]+$` and rejects everything else with a clear error, including `..` and path separators.

## 2. Unified Resolution

- [x] 2.1 Extract the explicit/current/default fallback precedence out of `resolveDeploymentDir` (`cmd/exasol/deployment_dir_resolution.go`) and `deploymentDirFromRawArgs` (`cmd/exasol/preregister_args.go`) into one shared function taking already-read flag values (deployment-dir value + changed, name value + changed) and returning `(config.DeploymentDir, source, error)`.
- [x] 2.2 Add `deploymentDirSourceNamed` to the source enum and wire the shared function to select it when `--deployment`/`-d` is explicit.
- [x] 2.3 Update `resolveDeploymentDir` and `deploymentDirFromRawArgs` to call the shared function, each supplying its own flag-reading (parsed `cmd.Flags()` vs. pre-scan `pflag.FlagSet`).
- [x] 2.4 Register `--deployment`/`-d` next to `--deployment-dir` in `registerDeploymentDirFlag` (or a renamed sibling function) so every command that currently gets `--deployment-dir` also gets `--deployment`/`-d`.
- [x] 2.5 Add `cmd.MarkFlagsMutuallyExclusive("deployment-dir", "deployment")` wherever both flags are registered.
- [x] 2.6 Call `cmd.ValidateFlagGroups()` as the first line of root's `PersistentPreRunE` (`cmd/exasol/root.go`), before `setupLogging()`, so mutual-exclusivity (and any future flag-group constraint) is enforced before resolution, compatibility enforcement, deployment file logging, or the version-update check run. Cobra's own later `ValidateFlagGroups()` call remains a harmless no-op in the success path.

## 3. Visibility

- [x] 3.1 Emit a terminal notice (via `addTerminalNotice`, stderr-safe) from the root pre-run when resolution selects the default directory or a named directory, replacing reliance on the `slog.Debug` line as the user-facing signal.
- [x] 3.2 Verify JSON-output commands keep valid JSON on stdout with the notice landing on stderr only.

## 4. Documentation and Help

- [x] 4.1 Update `rootCmdLongDesc` (`cmd/exasol/root.go`) and any other command help referencing `--deployment-dir` to mention `--deployment`/`-d` and the mutual-exclusivity rule.
- [x] 4.2 Update `README.md` deployment-directory section to describe `--deployment`/`-d` and named deployment directories, including that names are matched case-sensitively but may collide on case-insensitive filesystems (default macOS, Windows).

## 5. Tests and Verification

- [x] 5.1 Unit tests for `NamedDeploymentDirPath` and the `--deployment`/`-d` validator (accepted/rejected characters, `--deployment default`).
- [x] 5.2 Unit tests for the shared resolver: explicit `--deployment-dir`, explicit `--deployment`/`-d`, mutual exclusivity, current-directory precedence over default, explicit sources winning over current-directory.
- [x] 5.2a Test that passing both `--deployment-dir` and `--deployment`/`-d` fails before any side effect (no deployment log file created, no version-check triggered) — regression test for the `ValidateFlagGroups` ordering fix.
- [x] 5.3 Tests asserting `resolveDeploymentDir` and `deploymentDirFromRawArgs` agree on the same inputs.
- [x] 5.4 Tests for init/install creating a named deployment directory on first use, and for the stale-preset guard refusing a different preset in an initialized named directory (confirming it works with no additional wiring).
- [x] 5.5 Tests for `status --deployment <name>` (`-d <name>`) reporting the resolved path and `not_initialized` when applicable.
- [x] 5.6 Tests for the resolved-directory terminal notice appearing on stderr for both default and named fallback, and JSON stdout remaining valid.
- [x] 5.7 Run formatting, unit tests, and focused integration tests for deployment-directory resolution.

## 6. Deployment Listing (`exasol deployments list`)

This section is scoped to stand on its own; if it turns out more reviewable, it can ship as a separate follow-up PR after section 1-5 land.

- [x] 6.1 Add a `deployments` command group and `list` subcommand, following the `cache`/`cache list` pattern (`cmd/exasol/cache.go`) for structure and `--json` handling. Do not register `--deployment-dir` or `--deployment`/`-d` on either command — this is load-bearing, not cosmetic: it's what makes root's `PersistentPreRunE` skip deployment-directory resolution (and the new visibility notice) entirely for this command.
- [x] 6.2 Refactor `loadOrBackfillPresetIdentity` (`internal/deploy/preset_identity.go`) to stop persisting internally and rename it to `resolvePresetIdentity` — "load or backfill" implies the function itself performs the backfill/write, which is no longer true once persistence moves to the caller. It returns the derived-or-persisted `presetIdentityPair` plus a `backfilled bool`, with no `config.WriteExasolPersonalState` call. Update its sole existing caller, `EnsureDeploymentPresetIdentityMatches`, to perform the write itself when `backfilled` is `true`, preserving current behavior exactly. (A second, previously-unnoticed caller in `internal/deploy/configuration.go`'s `readDeploymentConfiguration` needed the same treatment to keep its existing backfill-and-persist behavior.)
- [x] 6.2a Add an exported `deploy.ResolveDeploymentPresetIdentity(deployment config.DeploymentDir) (PresetIdentityDisplay, error)` that reads state, calls the now-pure `resolvePresetIdentity`, and returns display-ready strings (via `presetIdentityDisplay`) — no write, ever. `deployments list` calls this. (Returns a small `PresetIdentityDisplay{Infrastructure, Installation string}` struct rather than two bare strings, to satisfy revive's confusing-results check without fighting nonamedreturns.)
- [x] 6.3 Implement the scan of `~/.exasol/personal/deployments/*`: skip non-directory entries (files, symlinks) outright; determine status via `isRecognizedDeploymentDir` (`cmd/exasol/deployment_dir_resolution.go`) — the same marker check (modern state file, deployment version marker, legacy `.workflowState.json`) already used for current-working-directory recognition — rather than a narrower check, so `deployments list` never disagrees with the rest of the CLI about whether a directory is a real deployment; use `deploy.ResolveDeploymentPresetIdentity` from 6.2a for initialized entries; surface a permission error reading the deployments root itself as a command failure, not an empty listing.
- [x] 6.4 Determine the active entry by calling the shared pure precedence function from section 2 directly (not `resolveDeploymentDirForCommand`, which also emits the visibility notice) with no `--deployment-dir`/`--deployment`/`-d` supplied, matching against the scanned entries; mark no entry active if the resolved active directory isn't among them (e.g. it's an external `--deployment-dir` path).
- [x] 6.5 Implement text and `--json` rendering, sorted alphabetically by name. (Empty JSON result is `[]`, matching `cache list --json`'s convention, not `null`.)
- [x] 6.6 Register the command (via its own `init()` in `cmd/exasol/deployments.go`, matching how every other command group registers itself) and document it in `README.md`.
- [x] 6.7 Test that `EnsureDeploymentPresetIdentityMatches` still backfills and persists identically to before the 6.2 refactor (same-preset reuse, different-preset refusal, and the on-disk write for old-style deployments) — a pure regression test for the split, independent of `deployments list`.
- [x] 6.8 Tests for `deployments list`: empty listing, non-directory entries ignored, mixed initialized/uninitialized entries, a legacy-marker-only entry reported as `initialized`, an old-style initialized entry showing a derived preset identity via `ResolveDeploymentPresetIdentity` with no state-file write occurring, active-entry detection from inside a listed recognized directory, from outside any, and from inside a recognized directory outside the listed tree, alphabetical ordering, JSON output shape.
