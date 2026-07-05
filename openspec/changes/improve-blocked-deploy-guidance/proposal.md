## Why

When `exasol install` or `exasol deploy` is re-run on a deployment whose current state does
not permit deployment (interrupted during a non-deploy operation, stopped, or a different
operation in progress), the command fails with a generic, low-level error:

```
ERR unexpected deployment status: run `status` for more information
Error: deployment failed: unexpected deployment status: run `status` for more information
```

This frames a *known, recoverable* state as "unexpected", never names which operation was
interrupted, and offers no direct call-to-action — it bounces the user to a separate
`exasol status` command to learn what to do. `exasol status` already computes exactly the
right per-state recovery guidance; the blocked command just fails to surface it.

## What Changes

- When `install`/`deploy` is blocked by the current deployment state, the command surfaces
  the same recovery guidance that `exasol status` produces (which operation was interrupted
  and the concrete recovery command to run), instead of the generic
  `unexpected deployment status` error.
- The blocked-state message names the resolved deployment directory and the current state.
- A valid, recoverable state (e.g. `interrupted`, `stopped`) is no longer presented to the
  user as an "unexpected" error.
- Recovery guidance stays single-sourced: the blocked path reuses the state→guidance
  mapping already used by `exasol status`, rather than duplicating the text.
- No lifecycle semantics change: the states in which deploy is permitted vs. blocked are
  unchanged; only the message shown when it is blocked improves.

## Capabilities

### Modified Capabilities

- `deployment-reconfiguration`: how `install`/`deploy` reports and guides recovery when the
  current deployment state does not permit deployment.

## Impact

- User-facing CLI behavior for `exasol install` and `exasol deploy` when the deployment is
  in a blocked state.
- No change to the set of states that permit deployment, to state transitions, or to any
  other command.
- Out of scope (candidate follow-up): the sibling guards in `connect` and the start/stop
  control commands share the same pattern and could adopt the same guidance later.
