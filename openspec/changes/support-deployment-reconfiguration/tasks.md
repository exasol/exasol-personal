## 1. State and Preset Identity

- [x] 1.1 Extend launcher state to persist infrastructure preset identity and installation preset identity.
- [x] 1.2 Add helpers to compare requested preset identity with persisted deployment preset identity.
- [x] 1.3 Add migration/backfill logic for existing initialized deployment directories without persisted preset identity.
- [x] 1.4 Add unit tests for built-in preset identity persistence, comparison, and backfill behavior.

## 2. Configuration Command

- [x] 2.1 Retire the top-level `exasol configure` command.
- [x] 2.2 Add `exasol config get [--json] [option...]` for inspecting active effective configuration.
- [x] 2.3 Add `exasol config set` with the same preset-specific `--option` flag style used by `init` and `install`.
- [x] 2.4 Add `exasol config reset [option...]` and `exasol config reset --all` for restoring defaults.
- [x] 2.5 Implement same-preset parameter patch/reset updates without deleting extracted presets, OpenTofu state, credentials, or connection metadata.
- [x] 2.6 Define and test allowed workflow states for `config set` and `config reset`, including initialized and deployment-failed retry states.
- [x] 2.7 Add help text and integration tests for `config get`, `config set`, and `config reset`.
- [x] 2.8 Keep backend workspace setup separate from configuration-only updates.
- [x] 2.9 Tell users that `config set` and `config reset` changes are applied by the next `deploy`.
- [x] 2.10 Refuse `config set` and `config reset` for running or stopped deployments with destroy-first guidance.
- [x] 2.11 Use structured deployment configuration values at the backend configuration boundary.

## 3. Init and Install Orchestration

- [x] 3.1 Update `init` so empty directories initialize presets and same-preset initialized directories do not apply configuration changes.
- [x] 3.2 Update `init` to refuse different requested presets with destroy/remove guidance.
- [x] 3.3 Update `install` to initialize empty directories, patch supplied same-preset options, and deploy.
- [x] 3.4 Update `install` to preserve local state and retry deploy for same-preset deployment-failed or deploy-interrupted states.
- [x] 3.5 Update deploy failure/status guidance to describe retry with `deploy` or same-preset `install` and cleanup with `destroy`.
- [x] 3.6 Keep initialized-directory orchestration in the command layer instead of inside initialization.

## 4. Local Removal

- [x] 4.1 Add `destroy --remove` flag and implement destroy-then-remove behavior.
- [x] 4.2 Ensure local removal preserves active deployment lock markers and runs only after successful cloud destruction.
- [x] 4.3 Add tests that failed destroy preserves local deployment files during `destroy --remove`.
- [x] 4.4 Add standalone local-only remove command for abandoned deployment directories.
- [x] 4.5 Remove the deployment directory itself and refuse non-deployment directories.
- [x] 4.6 Persist failed destroy as interrupted and show stale-destroy recovery guidance.
- [x] 4.7 Log local remove start and finish with deployment directory path.

## 5. Documentation and Verification

- [x] 5.1 Update README and command help for `init`, `config`, `install`, `destroy --remove`, and `remove`.
- [x] 5.2 Add integration tests for default-directory stale preset refusal and explicit local removal.
- [x] 5.3 Add integration tests for rerunning `install` with same presets after failed deployment state.
- [x] 5.4 Run formatting and focused Go unit tests.
- [x] 5.5 Run focused Python integration tests for init, install, config, destroy, and deployment-directory behavior.
- [x] 5.6 Keep final user guidance terminal-only and out of deployment logs.
- [x] 5.7 Print the EULA as terminal-only final notice and keep it out of deployment logs.
- [x] 5.8 Defer connection instruction output until after deployment log cleanup.

## 6. Review Feedback Follow-ups

- [x] 6.1 Always log the resolved deployment directory and its resolution source, not only the default case.
- [x] 6.2 Make the `remove` confirmation prompt spell out the exact deployment directory being removed.
- [x] 6.3 Make the `destroy --remove` confirmation prompt spell out the local deployment directory that will be removed after cloud destruction.
- [x] 6.4 Refuse `remove` (and `destroy --remove`'s local-removal step) when the current working directory is inside the deployment directory, with actionable guidance.
- [x] 6.5 Refuse `remove` (and `destroy --remove`'s local-removal step) when the running launcher binary is inside the deployment directory, with actionable guidance.
- [x] 6.6 Include the exact failing path in deletion errors when removing entries inside the deployment directory.
- [x] 6.7 Add Go unit and Python integration tests covering the new logging, prompt content, and safety guard rails.
- [x] 6.8 Refuse `config set` and `config reset` in every state in which cloud resources may already have been deployed (deployment-failed, interrupted during deploy or destroy, operation in progress, in addition to running and stopped), with destroy-or-remove guidance.
