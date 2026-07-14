## Context

Deployment-directory resolution currently has two independent implementations that must stay in lockstep:

- `resolveDeploymentDir` (`cmd/exasol/deployment_dir_resolution.go`), run from root `PersistentPreRunE` after Cobra has parsed flags. It checks `cmd.Flags()`/`cmd.InheritedFlags()` for an explicit `--deployment-dir`, falls back to a recognized current working directory, then the default directory.
- `deploymentDirFromRawArgs` (`cmd/exasol/preregister_args.go`), run *before* Cobra parses anything, using a throwaway `pflag.FlagSet` to pre-scan `os.Args`. This exists so `init`/`install`/`config set` can register the right dynamic infrastructure-variable flags before Cobra's real parse happens. It re-implements the same explicit/current/default fallback chain against raw args instead of a parsed `*cobra.Command`.

Both must recognize `--deployment`/`-d` identically, or `exasol config set --deployment foo <tab>` would offer the wrong dynamic flags relative to what the command actually resolves at runtime.

Everything downstream of resolution — the stale-preset-identity guard (`internal/deploy/preset_identity.go`), launcher-version compatibility (`cmd/exasol/compatibility.go`), and deployment file logging — reads only the resolved `config.DeploymentDir` and its on-disk contents. None of it branches on *how* the directory was resolved, so named directories inherit all of that behavior automatically. This was confirmed by tracing `EnsureDeploymentPresetIdentityMatches` and `enforceDeploymentDirectoryCompatibility`, which both take a `config.DeploymentDir` parameter only, never the resolution source.

Separately, the original default-deployment-directory change (`openspec/changes/archive/2026-05-28-add-default-deployment-directory`) specified: "Commands that resolve to the default deployment directory SHALL emit a clear human-facing message containing the resolved path." That was implemented as `slog.Info`, then later downgraded to `slog.Debug` by an unrelated commit ("keep diagnostic logs out of default command output"), which silently removed the only user-visible signal. No `addTerminalNotice` call was ever wired in. This change reinstates it properly, generalized to also cover the new named-directory case.

## Goals / Non-Goals

**Goals:**
- Let users select `~/.exasol/personal/deployments/<name>` via `--deployment <name>` (`-d <name>`) as easily as the default directory is selected implicitly.
- Keep `--deployment-dir` behavior and precedence completely unchanged for existing users and scripts.
- Collapse the two duplicated resolution implementations into one shared resolver.
- Make the resolved-directory visibility requirement actually true, for both the default and named cases.
- Let users discover what deployment directories already exist via `exasol deployments list`.

**Non-Goals:**
- Renaming or moving existing deployment directories.
- A cross-machine registry or sync of deployment names — this only enumerates `~/.exasol/personal/deployments/*` on the local machine.
- `exasol deployments prune` / `exasol deployments rename` — plausible future subcommands under the new `deployments` group, not built now.
- Migrating arbitrary `--deployment-dir` paths into the named scheme.

## Decisions

### `--deployment`/`-d` is a plain string flag, not a path

`--deployment-dir` uses `AbsDirVar`/`AbsDirValue`, which absolutizes and validates the argument as a path. `--deployment`/`-d` is a bare identifier that the resolver turns into a path itself (`filepath.Join(deploymentsDirName, name)` under the launcher root), so it gets its own `pflag.Value` that validates against `^[A-Za-z0-9_-]+$` and rejects everything else (including path separators and `..`) with a clear error at parse time, before any path construction happens.

Rationale:
- Keeps path-traversal prevention at the boundary where user input enters the system, matching how `AbsDirValue.Set` already validates eagerly rather than deferring to filesystem calls later.
- A restrictive allowlist is simpler to reason about and document than "anything except separators and `..`"; it can always be relaxed later if a real use case needs more characters, but cannot be tightened without breaking someone's existing named deployment.

`--deployment default` is accepted with no special-casing: it resolves to the exact same path as the implicit default, which is harmless and avoids an arbitrary reserved word.

### `--deployment-dir` and `--deployment`/`-d` are mutually exclusive, validated before any side effect runs

Registered via `cmd.MarkFlagsMutuallyExclusive("deployment-dir", "deployment")` wherever both are registered, so passing both is a usage error rather than requiring silent precedence rules.

By itself, `MarkFlagsMutuallyExclusive` is not sufficient. Checking `spf13/cobra@v1.10.2` (the version pinned in `go.mod`), `Command.execute()` runs flag parsing, then `PersistentPreRunE`, and only calls `c.ValidateFlagGroups()` — the method that actually enforces `MarkFlagsMutuallyExclusive` — afterwards. Root's `PersistentPreRunE` (`cmd/exasol/root.go`) is exactly where deployment-directory resolution, launcher-version compatibility enforcement, deployment file logging setup, and a version-update-check network call all happen. Left as just `MarkFlagsMutuallyExclusive`, a command invoked with both flags would resolve one of them, potentially open a deployment log file and make a network call, and only then get rejected — a rejected command should not have side effects.

The fix is general, not a bespoke duplicate check: call `cmd.ValidateFlagGroups()` ourselves as the first line of root's `PersistentPreRunE`, before `setupLogging()` or anything else. Flags are already fully parsed by the time `PersistentPreRunE` runs (`ParseFlags` happens earlier in `execute()`), so this is safe. Cobra will call `ValidateFlagGroups()` again later in `execute()`; calling it twice is harmless. This fixes the ordering for this flag pair and for any future `MarkFlagsRequiredTogether`/`MarkFlagsOneRequired` constraint added later, not just this one.

Rationale:
- Both flags answer the same question ("which deployment directory?"); there is no scenario where a user means to supply both, so silent precedence would only mask mistakes.
- Fixing the general ordering (validate flag groups before any `PersistentPreRunE` side effect) is more robust than adding a one-off mutual-exclusion check inside the resolver, and prevents the same class of bug for constraints added later.

### `--deployment`/`-d` sits at the same precedence tier as `--deployment-dir`, both above current-directory detection

Resolution order becomes: explicit `--deployment-dir` → explicit `--deployment`/`-d` → recognized current working directory → default directory. An explicit `--deployment`/`-d` wins even if the current directory happens to be a different recognized deployment directory, exactly like `--deployment-dir` already does.

Rationale:
- Consistency: a user should not need to remember that only one of the two explicit-selection flags overrides ambient current-directory state.

Alternatives considered:
- Let current-directory detection win over `--deployment`/`-d`. Rejected: it would make `--deployment`/`-d` unreliable precisely when it's most useful (switching between named deployments while sitting inside one of them).

### Unify the two resolvers around one pure precedence function

Extract the explicit/current/default(/named) fallback chain into a single function taking the already-known flag values (deployment-dir string + changed bool, name string + changed bool) and returning `(config.DeploymentDir, source, error)`. `resolveDeploymentDir` calls it with values read from `cmd.Flags()`; `deploymentDirFromRawArgs` calls it with values read from its pre-scan `pflag.FlagSet`. Only the "how do I read the flag" part stays duplicated (unavoidable — one reads a parsed `*cobra.Command`, the other a raw-args pre-scan); the actual precedence logic is written once.

Rationale:
- The two implementations already drifted apart once (subtly different in structure though not behavior); adding a third source without unifying first would guarantee a second drift.

Alternatives considered:
- Leave both implementations separate and add `--deployment`/`-d` handling twice. Rejected per explicit scope decision to clean this up now rather than compound the duplication.

### Generalize the stale-preset-reuse and compatibility guards by not touching them

Both `EnsureDeploymentPresetIdentityMatches` and `enforceDeploymentDirectoryCompatibility` key off the resolved directory's on-disk state, not the resolution source enum. No code changes are needed for named directories to get the same stale-preset and version-compatibility protection as the default directory. The `deployment-directory-resolution` spec's "Default deployment directory reuse SHALL avoid stale preset deployment" requirement is broadened in wording (not behavior) to describe both default and named directories, since the mechanism already covers both.

### Reinstate resolved-directory visibility as a real terminal notice, generalized to named directories

When resolution falls back to the default directory, or resolves via `--deployment`/`-d`, emit a message via the existing `addTerminalNotice`/`printTerminalMessages` mechanism (stderr-safe, keeps JSON stdout parseable) stating the resolved path. This replaces reliance on the `slog.Debug` line, which remains for structured log trails but is not sufficient as the user-facing signal.

Rationale:
- Closes a real, previously-unnoticed gap between the existing spec and the implementation.
- Named directories need the same visibility as the default directory for the same reason: users must be able to tell which deployment's state they are about to modify or destroy.

Alternatives considered:
- Only fix visibility for the new named case, leave the default-directory gap as-is. Rejected: the two cases share one code path and one notice mechanism; fixing only one would be arbitrary and leave the original spec still unfulfilled.

### `exasol deployments list` is a new command group, sequenced last

Modeled directly on the existing `cache`/`cache list` and `presets`/`presets list` pattern. It scans `~/.exasol/personal/deployments/*`, and for each directory entry (non-directory entries — stray files, symlinks — are ignored outright, not listed and not reported as errors) reports: name (the directory's leaf name), status (`initialized`/`not_initialized`), preset identity when initialized, absolute path, and whether it is the currently active deployment (i.e., what resolution would select right now with no flags from the current working directory). Entries are sorted alphabetically by name for deterministic, diffable output (filesystem directory iteration order is not guaranteed stable). Supports `--json` like `cache list` and `config get`.

Status is deliberately shallower than `exasol status`'s. `internal/deploy/status.go` defines seven status values (`not_initialized`, `initialized`, `operation_in_progress`, `interrupted`, `deployment_failed`, `database_connection_failed`, `database_ready`), computed by `deploy.Status()`, which acquires a per-directory lock. `deployments list` only ever reports `initialized`/`not_initialized`. This is intentional: running full lock-acquiring status resolution across every deployment directory would be slow and could contend with an operation already running against one of them. A listing command enumerating N directories should not risk blocking on, or interfering with, a deploy in progress in any one of them.

Status is determined via `isRecognizedDeploymentDir` (`cmd/exasol/deployment_dir_resolution.go`) — the same marker check already used for current-working-directory recognition elsewhere in the CLI (modern state file, deployment version marker, and the legacy `.workflowState.json` marker) — rather than a narrower ad hoc check. Using a different, narrower check (e.g. only the modern state file) would make `deployments list` disagree with the rest of the CLI about whether a given directory is a "real" deployment: a directory recognized as a deployment for cwd auto-detection and compatibility enforcement could show as `not_initialized` in the listing. Since `isRecognizedDeploymentDir` already lives in the same `cmd/exasol` package, this is a direct reuse, not a new export.

Preset identity display for an `initialized` entry reuses the manifest-derivation logic in `internal/deploy/preset_identity.go`, exported as a new read-only `ResolveDeploymentPresetIdentity` function once the existing derivation is split from its persistence step (see "Preset identity display must not write during a read-only list" below); a listing command must not carry a write side effect just to render a name.

A `deployments` group is justified over a flat `exasol list` because there are at least two other plausible future subcommands that only make sense with a bulk view across all deployment directories: `deployments prune` (remove stale/uninitialized directories, analogous to `cache clean`) and `deployments rename`. Neither is built in this change.

`deployments list` does not register `--deployment-dir` or `--deployment`/`-d` — it enumerates all deployment directories, so per-invocation directory selection does not apply. This is not just a UX simplification: root's `PersistentPreRunE` (`cmd/exasol/root.go`) resolves a deployment directory and emits the new resolved-directory visibility notice (see "Reinstate resolved-directory visibility..." below) only when the current command has registered one of these flags — `deploymentDirFlag(cmd)` returns `nil` otherwise, and `resolveDeploymentDirForCommand` no-ops. Registering either flag on `deployments list` would make every invocation resolve and print a stray "using default deployment directory" notice on top of the command's own dedicated output. The "which entry is active" computation (below) must therefore call the shared pure precedence function directly, never the notice-emitting wrapper.

This is deliberately the last section of `tasks.md` and does not entangle with the resolver/flag work, so it can be split into its own PR if that turns out to be more reviewable.

### Split preset-identity derivation from its persistence, instead of duplicating it

`loadOrBackfillPresetIdentity` (`internal/deploy/preset_identity.go:96-150`) is the only existing code that can name the preset for a deployment initialized before preset identity was persisted (an "old-style" deployment, still fully initialized, just missing `InfrastructurePresetIdentity`/`InstallationPresetIdentity` in its state file). Today it derives the name from the extracted manifests and unconditionally persists it back via `config.WriteExasolPersonalState` in the same call, upgrading the deployment to new-style in place. It has exactly one existing caller: `EnsureDeploymentPresetIdentityMatches`, run during `install`/`init`.

Rather than adding a second, near-duplicate function for `deployments list` to call, move the write out of `loadOrBackfillPresetIdentity` and into its one caller, renaming it to reflect what it does once persistence is no longer part of its contract:

- `loadOrBackfillPresetIdentity` is renamed to `resolvePresetIdentity` (kept private) and becomes a pure derivation: it returns the persisted pair unchanged when already present, or derives a pair from manifests when not — either way, no write — plus a `backfilled bool` indicating whether derivation (rather than a direct read) occurred. "Load or backfill" implied the function itself performed the backfill (i.e. persisted); once persistence moves to the caller, that name would be actively misleading. "Resolve" matches the vocabulary this codebase already uses for "figure out the value, deriving it if necessary, without implying a write" (`resolveDeploymentDir`, `ResolveInfrastructureConfigVariables`).
- `EnsureDeploymentPresetIdentityMatches` calls it, and persists via `config.WriteExasolPersonalState` itself only when `backfilled` is `true` — the exact same behavior as today, just with the write made explicit at the call site instead of buried inside the derivation. Its own name and doc comment (it does still ensure a match, and does still backfill on disk) are unaffected.
- A new exported `ResolveDeploymentPresetIdentity(deployment config.DeploymentDir) (PresetIdentityDisplay, error)` becomes the second caller: it reads state, calls the same pure `resolvePresetIdentity`, discards `backfilled`, and returns a display-ready `PresetIdentityDisplay{Infrastructure, Installation string}` (via `presetIdentityDisplay`) without ever writing. A named result struct is used instead of two bare strings to satisfy revive's confusing-results check without fighting nonamedreturns. `deployments list` calls this one.

Rationale:
- One derivation function, one behavior, reused by both callers — no drift risk between a "real" path and a "display" path that happen to compute the same thing slightly differently.
- The persisting caller already owns the decision to touch deployment state (it's already writing other things during `install`/`init`); moving the write there makes that ownership explicit instead of hidden inside a function named for loading.
- A command whose entire contract is "look, don't touch" (`deployments list`) never has a write path available to it at all, rather than merely "a version of the function that doesn't call it."
- The rename keeps the function's name honest about its actual contract after the split, rather than leaving a name that implies a side effect the function no longer has.

## Risks / Trade-offs

- [Reinstating a terminal notice on default/named fallback could be noisy for scripted/automated use] -> Emit on stderr only, never stdout; JSON output on stdout stays untouched, matching the original (unimplemented) design intent.
- [Unifying the two resolvers could subtly change existing default/current/explicit precedence if the extraction isn't careful] -> Keep the full existing test suite for both call sites green, and add tests asserting both call sites agree on the same inputs.
- [Scanning `~/.exasol/personal/deployments/*` for `deployments list` must tolerate partially-initialized directories] -> Treat a directory with no state file as `not_initialized` rather than failing the whole listing; ignore non-directory entries entirely; a permission error reading the deployments root itself is a genuine command failure, not an empty listing.
- [A restrictive name allowlist could reject a name someone already uses via `--deployment-dir`'s arbitrary path today] -> Not a breaking change: `--deployment-dir` is untouched and keeps accepting any path; the allowlist only constrains the new `--deployment`/`-d` flag.
- [Names are case-sensitive in the CLI (`Foo` ≠ `foo`), but on case-insensitive filesystems (default macOS, Windows) `--deployment Foo` and `--deployment foo` collide on disk despite looking distinct] -> Document this platform quirk in `README.md` rather than restricting the allowlist further; not worth the churn of forcing lowercase on an identifier users chose deliberately.
