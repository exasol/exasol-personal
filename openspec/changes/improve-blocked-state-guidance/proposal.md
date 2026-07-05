## Why

Several lifecycle commands refuse to run when the current deployment state does not permit
them, but they all fail the same unhelpful way. For `exasol install` / `exasol deploy`:

```
ERR unexpected deployment status: run `status` for more information
Error: deployment failed: unexpected deployment status: run `status` for more information
```

`exasol connect`, `exasol start`, and `exasol stop` behave identically when blocked. In
every case the command frames a *known, recoverable* state as "unexpected", never names
which operation was interrupted, and offers no direct call-to-action — it bounces the user
to a separate `exasol status` command to learn what to do. `exasol status` already computes
exactly the right per-state recovery guidance; the blocked commands just fail to surface it.

## What Changes

- When `install`/`deploy`, `connect`, `start`, or `stop` is blocked by the current
  deployment state, the command surfaces the same recovery guidance `exasol status`
  produces (which operation was interrupted and the concrete recovery command to run),
  instead of the generic `unexpected deployment status` error.
- The blocked-state message names the resolved deployment directory and the current state.
- A valid, recoverable state (e.g. `interrupted`, `stopped`, `initialized`) is no longer
  presented to the user as an "unexpected" error.
- Recovery guidance stays single-sourced: the blocked paths reuse the state→guidance
  mapping already used by `exasol status`, rather than duplicating the text.
- No lifecycle semantics change: the states in which each command is permitted vs. blocked
  are unchanged; only the message shown when it is blocked improves.

## Capabilities

### Modified Capabilities

- `deployment-reconfiguration`: how state-guarded lifecycle commands
  (`install`/`deploy`, `connect`, `start`, `stop`) report and guide recovery when the
  current deployment state does not permit the command.

## Impact

- User-facing CLI behavior for `exasol install`, `exasol deploy`, `exasol connect`,
  `exasol start`, and `exasol stop` when the deployment is in a blocked state.
- No change to the set of states that permit each command, to state transitions, or to any
  other behavior.
