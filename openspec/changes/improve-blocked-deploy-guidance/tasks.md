## 1. Surface recovery guidance on blocked deploy

- [x] 1.1 In `WorkflowStatePermitsDeploy` (`internal/deploy/deploy.go`), on every blocked
      state return an error that carries the resolved deployment directory, the current
      status, and the state-appropriate recovery guidance.
- [x] 1.2 Derive the recovery guidance from the same source `exasol status` uses
      (`GetStatus().Message`, backed by `buildInterruptMessage` /
      `staleOperationInProgressMessage`) — no duplicated guidance strings.
- [x] 1.3 Keep `ErrUnexpectedDeploymentStatus` / `ErrUnspportedOperation` as wrapped
      sentinels so `errors.Is` checks elsewhere keep working, but stop presenting the bare
      "unexpected deployment status" text as the user-facing message.
- [x] 1.4 Remove the now-redundant generic "run `status` for more information" wrapping on
      the deploy path so the actionable message is the one shown.

## 2. Verification

- [x] 2.1 Unit coverage: blocked deploy from `interrupted` (non-deploy operation) yields a
      message naming the state and the recovery command.
- [x] 2.2 Unit coverage: blocked deploy from `stopped` and from a foreign
      operation-in-progress yield state-appropriate guidance.
- [x] 2.3 Regression: `interrupted`-during-deploy and other permitted states still proceed
      (no behavior change).
- [x] 2.4 Run repository validation (`task fmt`, `task lint`, unit tests via `task
      tests-unit`; `tests-integration` needs cloud credentials and is left to CI).
