## Why

The launcher queues deployment-directory context during shared startup but normally prints queued messages only after the command returns. An interactive SQL session therefore shows its selected directory on exit. Other lifecycle messages have similarly surprising timing: automatic update checks run before commands, and `install` queues the EULA before deployment succeeds.

## What Changes

- Flush all terminal messages queued by shared startup immediately after deployment context resolution, before command-specific work.
- After a successful Cobra command lifecycle, perform the silent automatic update check where deployment state still exists, then flush remaining terminal messages with calls to action last. Failed commands do not check for or show automatic updates.
- Emit the EULA as a final operational notice only after successful `init` or the complete composite `install` succeeds.
- Preserve stderr notices, stdout results, JSON output, and existing error-specific recovery guidance.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `deployment-directory-resolution`: Require early, single delivery of the selected-directory notice as an effect of startup flushing.
- `cli-output-contract`: Define generic startup and successful-completion terminal flush phases.
- `launcher-version-check`: Run implicit checks only after eligible commands succeed and skip removed deployment state.
- `deployment-reconfiguration`: Keep the install EULA until the full install succeeds.

## Impact

Shared launcher lifecycle and terminal queues, version-check timing, install completion, Go and launcher tests, user documentation, and changelog.
