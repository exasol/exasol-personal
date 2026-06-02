## Context

Deployment directories contain extracted presets, resolved variable files, workflow state, OpenTofu state, generated credentials, and connection metadata. After the default deployment directory change, users commonly operate in `~/.exasol/personal/deployments/default`, so stale extracted presets can affect later commands even when the user supplies different `install` arguments.

The current `install` command runs `init` and then `deploy`. `init` treats an already initialized directory as success and does not compare the requested presets or variable overrides against the existing local state. This makes different-preset requests unsafe, but failed deployment retries need the opposite behavior: they must preserve local OpenTofu state because cloud resources may already exist.

## Goals / Non-Goals

**Goals:**
- Keep repeated `exasol install ...` retry-safe after failed or interrupted deploy attempts.
- Make same-preset configuration inspection and changes explicit through `exasol config get`, `exasol config set`, and `exasol config reset`.
- Let `exasol install ...` orchestrate `init`, configuration patching, and `deploy` so the common workflow remains convenient.
- Refuse different-preset requests and tell users to clean up explicitly before initializing a different preset.
- Persist preset identity so the launcher can make deterministic same-preset versus different-preset decisions.
- Keep destructive local cleanup isolated to `destroy --remove` and the standalone local-only `remove` command.
- Keep backend setup separate from configuration so configuration patch/reset operations remain parameter-only and backend implementations stay generic.

**Non-Goals:**
- Supporting in-place conversion between infrastructure presets.
- Supporting arbitrary migration of an active deployment from one preset to another.
- Deleting local state before cloud resources have been successfully destroyed.
- Removing local deployment directories without explicit user confirmation or an automation approval flag.

## Decisions

### Persist preset identity in launcher state

Initialization will persist the selected infrastructure preset identity and installation preset identity in `.exasolLauncherState.json`. Built-in presets use their preset name; path presets use the normalized path or a stable path token sufficient to compare future requests to the initialized directory.

Rationale:
- The launcher needs an authoritative source for whether a requested command targets the same deployment shape.
- Reading extracted manifests is useful but not enough for path presets or future manifest changes.
- State-based identity keeps command decisions independent of incidental file layout.

Alternatives considered:
- Infer identity from `infrastructure/infrastructure.yaml` and `installation/installation.yaml`. Rejected because it loses the original selector and is ambiguous for custom path presets.
- Compare entire extracted directory contents. Rejected because it is expensive, brittle, and still does not represent user intent.

### Keep `init` focused on preset extraction and same-preset patching

`init` will initialize an empty deployment directory by extracting presets and writing initial launcher state. When the directory is already initialized with the same presets, `init` will report that preset extraction is already complete. If the user supplied configuration flags, `init` applies those flags as a patch, equivalent to running `config set` after the same-preset initialization no-op. Omitted options keep their current effective values.

When the requested presets differ from the persisted preset identity, `init` will refuse and explain that users must run `destroy --remove` before initializing different presets, or `remove` if the cloud resources are already gone.

Command-level orchestration owns the initialized-directory decision: the lower-level initialization operation only creates fresh deployment state, while the command layer composes preset identity validation, initialization, and configuration patching.

Rationale:
- `init` remains about selecting and extracting presets, while still keeping the common repeated `init <preset> --option` workflow useful for same-preset configuration patches.
- Users get a clear distinction between "same deployment shape, change parameters" and "different deployment shape, clean up first".
- Refusing different presets by default prevents accidental cleanup or stale-state deployment.

Alternatives considered:
- Let `init` silently configure same-preset directories. Rejected because it hides a meaningful operation and makes explicit configuration commands less useful.
- Let `init` remove local state and reinitialize directly. Rejected because local removal must be coupled to successful cloud destruction when resources may exist.

### Add `config` subcommands as explicit same-preset configuration commands

`config get` will print the active effective configuration values for the already initialized preset. It supports plain terminal output and `--json`, and can restrict output to selected option names.

`config set` will patch deployment parameter files for the already initialized preset without deleting extracted presets, OpenTofu state, generated credentials, or workflow state. It exposes the same preset-specific `--option` style flags that apply to `init` and `install`, and omitted options keep their current values.

`config reset` will restore selected options, or all options when `--all` is passed, to preset defaults.

`config set` and `config reset` will refuse when the deployment is running or stopped. They are allowed in states where preserving resource state is required for retry, including initialized and deployment-failed deploy states. `config get` is read-only and can inspect initialized deployments regardless of lifecycle state.

Backend-specific configuration must be limited to writing configuration artifacts for the already initialized backend. It must not perform general workspace setup such as materializing backend tools.

Rationale:
- Users can intentionally inspect and change parameters without changing the preset.
- Failed deployment recovery can update a parameter, then retry `deploy` or `install`, while preserving partial resource state.
- The command group gives documentation and help a clear home for configuration-only behavior.

Alternatives considered:
- Keep configuration merged into `init`. Rejected because it makes `init` do two conceptually different things and makes same-preset mutation harder to explain.
- Require users to edit generated variable files manually. Rejected because it is error-prone and bypasses CLI validation.

### Separate backend setup from backend configuration

Backend implementations will expose separate lifecycle operations for workspace setup and configuration. `init` invokes setup after preset extraction and then writes configuration, while `config set` and `config reset` invoke only configuration. The interface remains generic so future backends can interpret setup and configuration in their own terms instead of inheriting OpenTofu-specific behavior.

Backend configuration uses structured deployment configuration values at the launcher/backend boundary instead of raw CLI override maps. Backend-specific adapters may still convert those values into their native representation internally.

Rationale:
- Configuration-only commands should not download, write, or otherwise prepare backend tooling.
- Retry after partial deployment must preserve backend state and only update requested parameters.
- The launcher orchestration can stay consistent across OpenTofu, local, and future backend implementations.
- Structured configuration values keep backend read and write operations aligned and avoid leaking command-specific flag parsing into backend interfaces.

### Make `install` a smart orchestration command

`install <preset> [options]` will choose the correct sequence based on deployment directory state and requested preset identity:
- Empty or uninitialized directory: run `init`, apply supplied configuration options, then `deploy`.
- Initialized directory with same presets: patch supplied configuration options, then `deploy`.
- Deployment failed or deploy-interrupted with same presets: preserve local state, patch supplied configuration options, then retry `deploy`.
- Initialized directory with different presets: refuse with destroy/remove guidance.

Rationale:
- Existing users can continue rerunning `install` after fixing permissions or other transient deployment problems.
- The same command becomes truthful: requested presets are checked, and requested parameters are applied when safe.
- `deploy` remains available for retry without configuration changes.

Alternatives considered:
- Make users switch to `deploy` after the first `install`. Rejected because existing workflow expectations already favor rerunning `install`.
- Make `install` always reinitialize. Rejected because it would destroy or corrupt the local state needed for cloud cleanup after partial failures.

### Use a local removal primitive for explicit cleanup

`destroy --remove` will destroy cloud resources using the current deployment state. Only after successful cloud destruction will it remove the local deployment directory. It must preserve the active lock marker while running and remove the directory after releasing the lock.

Rationale:
- Local files are removed only after the command no longer needs them for cloud cleanup.
- Local removal stays explicit and recoverable: `destroy --remove` for normal cleanup, `remove` for abandoned local state.

Alternatives considered:
- Add a destructive reinitialization flag to `init` and `install`. Rejected because it adds another destructive orchestration path, more flag-position complexity, and little UX benefit over telling users to run `destroy --remove` first.

### Provide a standalone local-only remove command for abandoned state

`remove` will delete the local deployment directory without attempting cloud destruction. It is a recovery command for cases where resources were already deleted manually, or the user no longer has access to destroy them through the launcher. It must require explicit confirmation unless `--auto-approve` is passed and refuse paths that do not look like Exasol Personal deployment directories.

Rationale:
- Users need a supported CLI path to recover a default deployment directory that is no longer destroyable.
- A top-level command is discoverable for users stuck with an unusable default deployment directory.
- The command name and confirmation text make clear that cloud resources are not destroyed.
- Removing the directory instead of emptying it keeps filesystem state consistent with a deleted local deployment.

Alternatives considered:
- Make `destroy --remove` continue removing local state after destroy fails. Rejected because it would discard the state needed to retry cleanup when resources might still exist.
- Keep local-only removal under `diag`. Rejected because users need a discoverable path when normal destroy is impossible.

### Make deployment-directory operations unambiguous in logs and prompts

The launcher will log the resolved deployment directory together with its resolution source (`explicit`, `current`, or `default`) for every command, not only when the default location is used. Confirmation prompts for `remove` and `destroy --remove` will spell out the exact local deployment directory that will be removed.

Rationale:
- Log trails always show the directory the launcher is operating on, even for support tickets where the resolution source is not obvious from CLI invocation alone.
- Users can sanity-check the path before approving a destructive operation, which reduces the chance of removing the wrong directory.

### Guard local removal against unsafe execution contexts

Before deleting any files, `remove` (and the local-removal step of `destroy --remove`) will refuse when:
- the current working directory equals or is nested inside the deployment directory, or
- the running launcher binary resides inside the deployment directory.

In both cases the launcher returns an actionable error naming the offending path, the deployment directory, and what the user should do (change directory, or move the binary).

Rationale:
- Windows rejects deleting a directory that contains the current process's working directory or a running executable, and the failure surface there is opaque; failing fast with a clear message is better than letting `os.RemoveAll` produce a generic OS error mid-operation.
- Users sometimes keep the launcher binary inside the deployment directory; the safety check prevents the launcher from removing itself.
- Other deletion failures (for example permission errors) now include the exact failing path so users can investigate without re-running with verbose logging.

Alternatives considered:
- Attempt deletion and only report on failure. Rejected because partial deletion may already have damaged state when the failure occurs.
- Restart the launcher from a temporary copy to delete itself. Rejected as out of scope and risky; the manual workaround (move/rename the binary) is simple and explicit.

### Refuse configuration changes whenever cloud resources may be deployed

`config set` and `config reset` (and the same patch path exercised by `install` when configuration overrides are supplied) only succeed in `initialized` state. Every other workflow state — `running`, `stopped`, `deploymentFailed`, `interrupted` (regardless of whether the interruption happened during a deploy or a destroy), and `operationInProgress` — is rejected with one consistent error that tells the user the deployment may already have cloud resources and points to `exasol destroy` (or `exasol remove` when the cloud resources are confirmed gone) as the way out.

Rationale:
- A local interruption (for example Ctrl-C during `exasol deploy`) does not prove that the cloud-side operation was aborted; provider APIs may still have provisioned or partially provisioned resources. Allowing configuration changes in that state risks the next `deploy` mutating live infrastructure with parameters the user never confirmed against the existing setup.
- The same reasoning applies to `deploymentFailed`: a partial deploy can leave provider resources behind whose configuration is now out of sync with whatever new values would be written.
- A single rule (`initialized only`) is far easier to reason about than per-state allowlists, both for users reading the error and for future maintenance.

Trade-off:
- Users who want to retry a failed or interrupted deploy with different configuration must run `exasol destroy` first (or `exasol remove` if they have already verified no cloud resources exist). Pure retries without configuration changes still work via `exasol deploy` or `exasol install <same-presets>`, both of which remain permitted from `deploymentFailed` and deploy-interrupted states.

## Risks / Trade-offs

- [Dynamic flags for `config set` depend on initialized deployment state] -> Resolve the deployment directory and load stored preset manifests before final Cobra parsing for `config set`, similar to current pre-registration of `init` and `install` preset variables.
- [Path preset identity comparison can be surprising if files change in place] -> Compare the normalized selected path and document that path preset contents are treated as the same preset identity until explicit cleanup is performed.
- [Local removal can remove diagnostic files users wanted to keep] -> Document `destroy --remove` clearly and keep plain `destroy` preserving local state.
- [Failed deploy status text currently points users toward destroy before retry] -> Update status and failure hints to make retry and cleanup paths explicit.

## Migration Plan

Existing initialized deployment directories will not have persisted preset identity. The first command that needs identity should derive it from existing extracted manifests where possible and persist it, or return an actionable compatibility error if identity cannot be determined safely.

Rollout should preserve plain `destroy` behavior, add `destroy --remove`, and update `install` behavior only after identity persistence and same-preset configuration are implemented.

## Open Questions

- For custom path presets, is normalized path identity sufficient, or do we also need a content fingerprint for better diagnostics?
