## Why

The only way to run more than one deployment side by side today is `--deployment-dir <path>`, which forces users to invent and remember an arbitrary filesystem path outside the launcher's own layout. There is no lightweight way to say "give me a second deployment, distinct from the default one" without leaving the launcher-managed `~/.exasol/personal/deployments/` tree, and no way to see what deployments already exist there.

## What Changes

- Add a `--deployment <name>` flag (shorthand `-d <name>`) as a peer of `--deployment-dir`. It selects `~/.exasol/personal/deployments/<name>` instead of the `default` directory. `--deployment-dir` and `--deployment` are mutually exclusive.
- Validate `<name>` against a restrictive allowlist (letters, digits, dash, underscore) since it becomes a literal directory name. `--deployment default` is valid and simply resolves to the same path as the implicit default.
- Extend deployment-directory resolution precedence to: explicit `--deployment-dir`, explicit `--deployment`/`-d`, recognized current working directory, then the default directory. Both existing resolution call sites (root pre-run resolution and the pre-Cobra flag pre-scan used for dynamic infrastructure-variable flags) are unified onto one shared resolver instead of maintaining two parallel implementations.
- Fix a pre-existing gap where falling back to the default deployment directory has no actual human-facing message (only an unused debug log survives from the original default-directory change). Add a real terminal notice for both the default-directory fallback and the new named-directory selection, on stderr so JSON stdout stays parseable.
- Add `exasol deployments list`, a new command group enumerating deployment directories under `~/.exasol/personal/deployments/`, showing each one's name, initialization status, preset identity (when initialized), path, and which one is currently active. Supports `--json`.
- Update root command help and `README.md` to document `--deployment`/`-d` alongside `--deployment-dir`.

The stale-preset-reuse guard and the launcher-version compatibility check both already operate purely on the resolved `config.DeploymentDir` value, independent of how it was resolved — named deployment directories inherit that protection automatically, with no additional wiring.

## Capabilities

### New Capabilities
- `deployment-directory-listing`: `exasol deployments list` enumerates known deployment directories, their initialization status, preset identity, and which one is active.

### Modified Capabilities
- `deployment-directory-resolution`: add `--deployment`/`-d` as an explicit resolution source, unify the two resolver implementations, and implement the previously-unfulfilled resolved-directory visibility requirement for both default and named directories.

## Impact

- Affected CLI flag registration (`cmd/exasol/commonFlags.go`) across every command that currently registers `--deployment-dir` (~20 commands).
- Affected deployment-directory resolution (`cmd/exasol/deployment_dir_resolution.go`, `cmd/exasol/preregister_args.go`) and the deployment-directory layout helper (`internal/config/deployment_dir.go`).
- Affected root pre-run terminal messaging (`cmd/exasol/root.go`) for the new visibility notices.
- New command group and files for `exasol deployments list`.
- Affected `internal/deploy/preset_identity.go`: `loadOrBackfillPresetIdentity`'s persistence step moves to its existing sole caller (`EnsureDeploymentPresetIdentityMatches`, behavior-preserving) and it is renamed to `resolvePresetIdentity` to match its new, write-free contract; a new exported read-only `ResolveDeploymentPresetIdentity` function is added for `deployments list` to display preset names without writing to deployment state.
- Documentation and help text updates.
- No new external dependencies. No change to existing `--deployment-dir` behavior or existing deployment directories.
