## Why

Linux local deployments run the database on the user's own machine, where the
deployment host is already reachable. Serving `exasol shell host` there gives
one consistent way to inspect a deployment host across platforms, and gives
later work a supported path for running explicit host commands.

Related issue: [SPOT-32683](https://exasol.atlassian.net/browse/SPOT-32683).

## What Changes

- Run the shell named by `SHELL` for `exasol shell host` on Linux local
  deployments, falling back to `/bin/sh`.
- Execute explicit commands after `--` directly, preserving their argument
  boundaries, environment, working directory, and standard streams.
- Accept optional child-process environment overrides on the command contract
  shared by direct, runner, and SSH execution.

## Capabilities

### Modified Capabilities

- `exasol-local-deployment`: Linux local host shell and command execution.

## Impact

The change affects the host shell command, the Linux local runtime, and the
execution environments shared by local and remote command transports.

User documentation, the generated CLI reference, and the unreleased changelog
describe Linux host access.
