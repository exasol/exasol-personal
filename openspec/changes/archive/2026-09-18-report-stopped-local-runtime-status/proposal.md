## Why

On Windows the deployment container runs inside Podman's machine, and that machine does not
come back automatically after the host reboots. A deployment recorded as running is therefore
routinely found with its machine stopped, and `exasol status` answers:

```
Deployment directory: C:\Users\me\.exasol\personal\deployments\default
Status: database_connection_failed
```

That is the whole output. The user is told the database cannot be reached but not that the
deployment is merely stopped, and not that `start` brings it back. Linux reports
`stopped` with recovery guidance in the same situation, because its container probe answers
cleanly when the container is absent; on Windows the probe cannot reach the machine at all,
and an unreachable probe is currently indistinguishable from a probe that says "not running".

`database_connection_failed` is also the one status the launcher produces with no message,
so even when the runtime genuinely is running the user gets a bare status word.

## What Changes

- `exasol status` reports a local deployment as `stopped`, with the existing guidance to run
  `start` or `destroy`, when the environment hosting its container is not running.
- `exasol status` answers that without starting the environment, so inspecting a deployment
  stays free of side effects and stays inside the status timeout.
- `exasol status` accompanies `database_connection_failed` with recovery guidance, so every
  status it reports tells the user what to do next.

## Capabilities

### Modified Capabilities

- `exasol-local-deployment`: report a local deployment as stopped when the environment
  hosting its container is not running, without starting that environment.
- `deployment-lifecycle-recovery`: accompany an unreachable-database status with recovery
  guidance.

## Impact

User-visible `exasol status` output for local deployments, on Windows after the Podman
machine stops and on every platform when the database is unreachable. The status of a
running deployment, the set of status values, the status timeout, and the start, stop,
deploy, and destroy workflows are unaffected.
