# Design

## Context

`install` and `deploy` both funnel through `deployFromManifests` →
`WorkflowStatePermitsDeploy` (`internal/deploy/deploy.go`). That guard permits deployment
from `initialized`, `deployment_failed`, `running`, an in-progress *deploy* operation, and
an *interrupted-during-deploy* state (tofu apply is idempotent). Every other state is
blocked and today returns a bare sentinel:

- interrupted during a non-deploy operation, or `stopped` → `ErrUnexpectedDeploymentStatus`
- a foreign operation in progress → `ErrUnspportedOperation`

The caller wraps it as `run \`status\` for more information`, and `install.go` prefixes
`deployment failed:`. Net user output frames a valid state as "unexpected" and gives no
direct next step.

Meanwhile `GetStatus` (`internal/deploy/status.go`) already maps each workflow state to a
concise, actionable message via `buildInterruptMessage` and
`staleOperationInProgressMessage` — e.g. *"Interrupted during destroy. Please run
`destroy`."*, *"Deployment stopped. Run `start` to restart or `destroy` to delete
resources."* This is the guidance the blocked path should show.

## Decision

On a blocked deploy, build the error from `GetStatus(ctx, deployment, /*checkConnection=*/false)`
so the guidance is single-sourced. The message names the resolved deployment directory and
the current state, then appends `status.Message`:

```
deployment in <dir> is in state "interrupted" and cannot be deployed right now.
Interrupted during destroy. Please run `destroy`.
```

- Wrap the existing sentinel (`ErrUnexpectedDeploymentStatus` for interrupted/stopped,
  `ErrUnspportedOperation` for a foreign in-progress operation) with `%w` so `errors.Is`
  checks in `connect.go` / `deploymentControl.go` and any future callers keep working — but
  the sentinel's own terse text is no longer the salient part of what the user reads.
- Drop the generic `run \`status\` for more information` wrapper on this path; the message
  is now self-contained and actionable.
- `checkConnection=false` keeps the guard fast and side-effect free (no DB probe), matching
  the existing `LogDeploymentStatus` behavior it replaces.

## Alternatives considered

- **Duplicate the recovery strings at the deploy site.** Rejected: drifts from `status`
  and violates single-source (the user's explicit steer).
- **Change `ErrUnexpectedDeploymentStatus`'s message text.** Unnecessary: the sentinel is
  shared by other guards; wrapping with a rich message is enough and lower-risk.
- **Point users to `exasol info` instead.** `info` guidance is a separate, in-progress
  change; the direct recovery command is more actionable here than another redirect.

## Risks

- Low. Behavior of which states permit deploy is unchanged; only the blocked-path message
  changes. If `GetStatus` itself errors, fall back to returning the wrapped sentinel so the
  guard never masks a real read failure.
