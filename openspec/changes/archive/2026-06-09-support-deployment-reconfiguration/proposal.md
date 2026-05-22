## Why

The new default deployment directory makes repeated `exasol install` calls reuse the same local deployment state by default, but the launcher currently ignores new preset arguments once a directory is initialized. Users can accidentally request one preset while the launcher deploys a previously initialized preset, and retry workflows after partial deployment failures must preserve local infrastructure state.

## What Changes

- Add explicit `config get`, `config set`, and `config reset` commands for inspecting, patching, and resetting parameters of an already initialized deployment without replacing presets or deleting local state.
- Make `init` initialize empty directories, patch supplied options for same-preset initialized directories, and refuse incompatible initialized directories instead of silently ignoring requested presets.
- Make `install` orchestrate initialization, configuration, and deployment in a state-aware way so rerunning the same install command remains retry-safe.
- Refuse different requested presets with guidance to run `destroy --remove` first, or `remove` when cloud resources are already gone.
- Add `destroy --remove` to remove the local deployment directory only after cloud resources have been successfully destroyed.
- Add `remove` as a recovery command for removing the local deployment directory when resources were already deleted manually or can no longer be destroyed through the launcher.
- Persist selected preset identity in launcher state so commands can distinguish same-preset configuration updates from different-preset refusal.
- Update status/help/documentation so users understand when to use `config`, when `install` retries are safe, and when `destroy --remove` or `remove` is required.

## Capabilities

### New Capabilities
- `deployment-reconfiguration`: CLI behavior for initializing, configuring, retrying, replacing, and removing local deployment state while preserving cloud cleanup safety.

### Modified Capabilities
- `deployment-directory-resolution`: Existing default-directory behavior is updated so default deployment directories can be safely reused or configured without silently deploying stale preset contents.

## Impact

- Affected CLI commands: `init`, new `config`, `install`, `deploy`, `destroy`, `remove`, `status`, and related help text.
- Affected deployment state model: launcher state must persist selected infrastructure and installation preset identity and configuration lifecycle semantics.
- Affected local filesystem handling: `destroy --remove` removes the local deployment directory after successful cloud destruction, and `remove` removes abandoned local state without cloud destruction.
- Affected retry behavior: failed or interrupted deploy attempts must preserve local infrastructure state and allow same-preset retries through `install` and `deploy`.
- Documentation and integration tests must cover default-directory reuse, same-preset configuration changes, different-preset refusal, and local removal behavior.
