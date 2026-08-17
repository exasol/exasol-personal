## Why

Users and automation need `exasol info` to be the obvious place to discover deployment state and connection-relevant details. The command previously failed too early for missing deployments and the machine-readable output was not a clear public contract for deployment discovery.

## What Changes

- Make `exasol info` useful before deployment initialization by reporting the resolved deployment directory, the `not_initialized` state, and clear next steps.
- Make `exasol info --json` a stable machine-readable deployment overview for both uninitialized and initialized deployment directories.
- Keep human-readable `exasol info` as the default experience, with state-specific next steps that help users choose the next command.
- Ensure machine-readable output stays valid JSON on stdout and does not include terminal-only prose or prompts.
- Preserve existing lifecycle semantics: this change reports deployment state and guidance but does not create, modify, start, stop, or destroy deployments.

## Capabilities

### New Capabilities

- `deployment-info-reporting`: how users and automation discover deployment state, next steps, and connection-relevant information through `exasol info`.

### Modified Capabilities

<!-- None: no existing permanent spec covers deployment information reporting. -->

## Impact

- User-facing CLI behavior for `exasol info` and `exasol info --json`.
- Automation that reads `exasol info --json` can rely on structured deployment state and connection-relevant fields.
- Existing deployment lifecycle commands and connection semantics are unchanged.
