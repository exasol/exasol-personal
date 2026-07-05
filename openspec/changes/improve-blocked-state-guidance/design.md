# Design

## Context

Four lifecycle commands guard on the workflow state before acting, and all fail the same
way when the state does not permit them:

- `install`/`deploy` → `WorkflowStatePermitsDeploy` (`internal/deploy/deploy.go`)
- `connect` → `WorkflowStatePermitsConnect` (`internal/deploy/connect.go`)
- `start` → `WorkflowStatePermitsStart` (`internal/deploy/deploymentControl.go`)
- `stop` → `WorkflowStatePermitsStop` (`internal/deploy/deploymentControl.go`)

Each blocked path logged the status and returned a bare sentinel
(`ErrUnexpectedDeploymentStatus` or `ErrUnspportedOperation`); the caller wrapped it as
`run \`status\` for more information`, and (for install) `install.go` prefixed
`deployment failed:`. Net user output framed a valid, recoverable state as "unexpected" and
gave no direct next step.

Meanwhile `GetStatus` (`internal/deploy/status.go`) already maps each workflow state to a
concise, actionable message via `buildInterruptMessage` and
`staleOperationInProgressMessage` — e.g. *"Interrupted during destroy. Please run
`destroy`."*, *"Deployment stopped. Run `start` to restart or `destroy` to delete
resources."*, *"Ready for deployment. Run `deploy` ..."* This is the guidance every blocked
command should show, and it is command-agnostic: it describes how to move the deployment
out of its current state regardless of which command the user typed.

## Decision

Add one shared helper, `newBlockedStateError(deployment, sentinel)` in `shared.go`, used by
all four guards. It builds the message from
`GetStatus(ctx, deployment, /*checkConnection=*/false)` so the guidance is single-sourced:

```
deployment in <dir> is in state "interrupted". Interrupted during destroy. Please run `destroy`.
```

- The helper returns a `blockedStateError` that wraps the sentinel via `Unwrap()` (so
  `errors.Is` checks keep working) while fully controlling the user-facing text — the
  sentinel's terse "unexpected deployment status" no longer leads the message.
- The redundant `run \`status\` for more information` wrappers on each guard's call site are
  removed; the message is now self-contained.
- `checkConnection=false` keeps the guards fast and side-effect free (no DB probe), matching
  the `LogDeploymentStatus` behavior it replaces. `LogDeploymentStatus` had no other callers
  and is removed.

## Alternatives considered

- **Duplicate the recovery strings at each guard.** Rejected: drifts from `status` and
  violates single-source.
- **Change `ErrUnexpectedDeploymentStatus`'s message text.** Unnecessary and broader: the
  sentinel is shared; wrapping with a rich message via a custom error type is enough and
  lower-risk.
- **Fix only `install`/`deploy`.** Rejected: `connect`, `start`, and `stop` share the exact
  same dead-end and would reproduce the same complaint; one shared helper fixes all.

## Risks

- Low. Which states permit each command is unchanged; only the blocked-path message
  changes. If `GetStatus` itself errors, the helper falls back to the wrapped sentinel so a
  guard never masks a real read failure.
