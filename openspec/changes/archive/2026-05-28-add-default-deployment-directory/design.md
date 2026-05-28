## Context

Deployment-directory selection is currently implicit in the `--deployment-dir` flag default: commands that register the flag receive the current directory as their deployment directory unless the user overrides it. That makes the default journey require users to create or remember a directory before installing, and it also means the CLI cannot clearly distinguish an omitted flag from an explicit directory choice.

The implementation is cross-cutting because deployment-directory selection affects command pre-run validation, compatibility checks, deployment file logging, version update checks, status formatting, and documentation. The deployment directory remains the local source of truth for state; this change only changes how the CLI chooses that directory when the user did not provide one.

## Goals / Non-Goals

**Goals:**
- Make the default workflow use `~/.exasol/personal/deployments/default` when no explicit directory or recognized current deployment directory is available.
- Preserve explicit `--deployment-dir` and multiple deployment directory workflows.
- Keep the selected directory visible in user-facing output, especially `status`.
- Let `status` report `not_initialized` for uninitialized resolved directories.
- Keep commands that require initialized state failing with actionable path-aware errors.
- Avoid breaking machine-readable JSON output with human-only diagnostic messages.

**Non-Goals:**
- Adding commands for deleting deployment directories.
- Moving deployment state out of deployment directories.
- Introducing a global registry of deployments.
- Migrating existing deployment directories into the default directory.

## Decisions

### Resolve the deployment directory once in the command layer

The CLI will resolve the active deployment directory during root pre-run before compatibility enforcement, deployment file logging, version update hints, and command execution. The resolved path is written back to the common command state so existing command implementations can continue using the common deployment accessor.

Rationale:
- Resolution is a CLI concern because it depends on command flags, current working directory, and user-facing diagnostics.
- Resolving once prevents commands from independently making different choices.
- Writing the resolved value back keeps the implementation localized and avoids threading a new parameter through all commands.

Alternatives considered:
- Resolve independently inside each command. Rejected because it would duplicate precedence logic and risk inconsistent behavior.
- Move resolution into the lower-level deployment package. Rejected because that package should not depend on CLI flag semantics or terminal messaging.

### Treat explicit `--deployment-dir` as the highest-precedence signal

The resolver will detect whether the deployment directory flag was explicitly provided and use that path when present, even if the current directory is also a deployment directory.

Rationale:
- Existing scripts using `--deployment-dir` must remain stable.
- Explicit command-line input should override ambient current-directory state.

Alternatives considered:
- Prefer the current deployment directory when already inside one. Rejected because it would make explicit flags unreliable.

### Recognize current deployment directories by launcher-owned markers

When no explicit flag is present, the resolver will use the current working directory if it contains launcher-owned deployment state or marker files. This includes current state markers and legacy markers so that incompatible or older directories still flow into compatibility checks and produce the intended errors instead of being silently ignored.

Rationale:
- Users who `cd` into an existing deployment continue to manage that deployment.
- Broken or legacy deployment directories should not be hidden by falling back to the default directory.
- Random empty directories should not capture the default workflow.

Alternatives considered:
- Treat any current directory as the deployment directory. Rejected because it preserves the friction this change is removing.
- Treat only fully compatible deployment directories as current deployment directories. Rejected because it could silently bypass legacy or damaged deployment directories.

### Keep the default directory transparent but not noisy in JSON stdout

Commands that resolve to the default directory will emit a clear human-facing message containing the resolved path. The message should use stderr or the existing logging path rather than stdout payload output.

Rationale:
- Users must be able to see where state is stored.
- JSON output needs to remain parseable by automation.

Alternatives considered:
- Print the default-directory message to stdout for all commands. Rejected because it would corrupt JSON output.
- Only show the default path in `status`. Rejected because install and other operations should make the state location visible immediately.

### Make `status` valid for uninitialized resolved directories

The status command will no longer require an initialized deployment directory before it runs. It will include the active deployment directory in text and JSON output and report `not_initialized` when the resolved directory has no initialized deployment state.

Rationale:
- `status` is a discovery command; failing solely because initialization has not happened is less useful than reporting that state.
- Showing the active path in `status` gives users a consistent way to learn which directory a command context points at.

Alternatives considered:
- Keep `status` requiring initialized state. Rejected because it conflicts with the goal that commands produce useful messages in all circumstances.

## Risks / Trade-offs

- [Users who create an empty directory and run `exasol install` without `--deployment-dir` will use the default directory] -> Document the new default journey and explain that `--deployment-dir .` explicitly selects the current empty directory.
- [Default-directory diagnostics could break command output consumers] -> Emit human-facing diagnostics on stderr/logging and keep JSON stdout valid.
- [A current directory with partial launcher state could be selected and then fail compatibility] -> Preserve that failure because it is safer than silently operating on a different deployment.
- [Status on a missing default directory could accidentally create state] -> Prefer reporting `not_initialized` without creating the directory; creation remains owned by init/install flows.
