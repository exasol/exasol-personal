## 1. Surface recovery guidance on blocked lifecycle commands

- [x] 1.1 Add a shared `newBlockedStateError` helper (`internal/deploy/shared.go`) that
      builds an error carrying the resolved deployment directory, the current status, and
      the state-appropriate recovery guidance.
- [x] 1.2 Derive the recovery guidance from the same source `exasol status` uses
      (`GetStatus().Message`, backed by `buildInterruptMessage` /
      `staleOperationInProgressMessage`) — no duplicated guidance strings.
- [x] 1.3 Use the helper in every state guard: `WorkflowStatePermitsDeploy`
      (`deploy.go`), `WorkflowStatePermitsConnect` (`connect.go`),
      `WorkflowStatePermitsStart` and `WorkflowStatePermitsStop` (`deploymentControl.go`).
- [x] 1.4 Keep `ErrUnexpectedDeploymentStatus` / `ErrUnspportedOperation` as wrapped
      sentinels so `errors.Is` checks keep working, but stop presenting the bare
      "unexpected deployment status" text as the user-facing message.
- [x] 1.5 Remove the now-redundant generic "run `status` for more information" wrapping and
      the unused `LogDeploymentStatus` helper.

## 2. Verification

- [x] 2.1 Unit coverage: blocked `deploy` from `interrupted` (non-deploy), `stopped`, and a
      foreign operation-in-progress yield state-appropriate guidance.
- [x] 2.2 Unit coverage: blocked `connect`, `start`, and `stop` surface recovery guidance
      naming the state and directory.
- [x] 2.3 Regression: permitted states (e.g. `deploy` when interrupted-during-deploy,
      `connect` when running) still proceed.
- [x] 2.4 Run repository validation (`task fmt`, `task lint`, `task all`).
