## 1. Bound the local readiness waits

- [x] 1.1 Apply `waitTimeoutSeconds` in `internal/deploy/local_backend.go`'s `Start` (currently ignored via `_ int`) using `context.WithTimeout`, mirroring the tofu backend's existing pattern.
- [x] 1.2 Give `Deploy` (used by `exasol install local`) an equivalent bounded wait for its readiness step, using a default timeout constant since its interface has no caller-supplied timeout. Added `LocalDatabaseStartedDefaultTimeoutSeconds` in `shared.go`; threaded a `waitTimeoutSeconds` parameter through `deployLocalRuntime`/`startLocalRuntime`/`startPreparedLocalRuntime`/`writeLocalRuntimeArtifactsAndWait`, applied via `context.WithTimeout` at the point the database wait starts.

## 2. Consume the runner's health-check contract

- [x] 2.1 Add a client method in `internal/localruntime` for the runner's `{"request":"health-check"}` status-socket request, returning a per-port classified state (`reachable`/`refused`/`blocked`/`timeout`) for every forwarded port. Treat this as an external contract, with no dependency on runner-internal implementation. Added `HealthCheck`, `PortState`, `PortHealth`, `HealthCheckResult` in `internal/localruntime/runtime.go`, invoking the runner's `health-check` CLI subcommand (the launcher only shells out to the runner binary and never dials its socket directly).
- [x] 2.2 Handle a missing/unsupported/undecodable `health-check` response by falling back to the caller's original error rather than erroring, so an old already-running runner daemon degrades gracefully to today's generic behavior.
- [x] 2.3 Unit test the client against a fake runner: a mixed per-port response (one reachable, one blocked) parses correctly, and a runner that doesn't recognize the request returns an error rather than a zero-value result.

## 3. Classify reachability failures

- [x] 3.1 Add a typed error (mirroring `blockedStateError` in `internal/deploy/shared.go`: `Error()` returns the full actionable message, `Unwrap()` exposes a sentinel) representing a local runtime reachability problem. `localReachabilityError`/`ErrLocalReachability` in new file `internal/deploy/local_reachability.go`.
- [x] 3.2 Implement the differential-diagnosis rule: every forwarded port non-reachable classifies as a reachability error; any single port reachable defers to the existing generic failure, unchanged. `classifyLocalReachability`.
- [x] 3.3 Wire the classification into `internal/deploy/ready.go`'s readiness wait for install/start as a fast pre-check, so a persistently network-wide-blocked deployment fails immediately rather than after the full backoff window.
- [x] 3.4 Wire the same classification into the local runtime's own `start` invocation (`internal/deploy/local_runtime.go`), since the runner's internal SSH-readiness gate can fail before the SQL-readiness wait is ever reached.
- [x] 3.5 Wire the same classification into `exasol connect` (`internal/deploy/connect.go`) and `exasol shell host`/`exasol shell container` (`internal/deploy/local_backend.go`'s `OpenHostShell`/`OpenCOSShell`) on failure.
- [x] 3.6 Write the reachability error's message content: names example invoking environments (terminal emulator, editor, agent host) as the macOS Local Network permission target, and explains the loopback nuance (permission required even though the endpoint is `127.0.0.1`, since the launcher forwards it from a VM).
- [x] 3.7 Unit test `classifyLocalReachability`: every port blocked classifies as a reachability error; only the database port blocked defers to the generic failure; a non-local deployment and an unavailable health-check both no-op (defer to the caller).

## 4. Fix `exasol connect`'s missing dial timeout

- [x] 4.1 Thread `ctx` through to the actual driver dial in `internal/connect/exasol/database.go`'s `Connect`, which previously ignored it. Added `connectWithContext`, which runs the (context-less) underlying dial call in a goroutine and selects on it vs. `ctx.Done()`.
- [x] 4.2 Apply a bounded timeout to that dial so a non-reachability failure also fails predictably instead of hanging indefinitely. Added `connectDialTimeout` (30s) in `internal/connect/connect.go`.

## 5. `exasol diag local` command

- [x] 5.1 Add the `exasol diag local` command surface. `cmd/exasol/diagLocal.go`, mirroring the existing `diag info` command's structure (always-JSON output, version-compatibility and initialized-deployment guards, deployment-dir flag).
- [x] 5.2 Report local VM status, reported guest IP, and bound host ports from existing runner state. `internal/deploy/diag_local.go`'s `diagnoseLocalUnsafe`.
- [x] 5.3 Report per-port reachability via the health-check client from section 2.
- [x] 5.4 Report database SQL-level readiness (a single check, without the polling/backoff wrapper used elsewhere).
- [x] 5.5 Report current OS/architecture support for the local preset, usable even without a running deployment.
- [x] 5.6 On a supported platform with no VM running, report a concise "ready to run" message with the instruction to start it, instead of reachability/readiness checks that don't apply yet.
- [x] 5.7 Report a warning when the local VM is running but the recorded deployment workflow state doesn't expect one to be (e.g. a process orphaned by a prior crash), since this can cause a future `start`/`install` to fail with a VM storage conflict.
- [x] 5.8 Test the command's output for: unsupported platform, non-local deployment, VM not running, VM running with ports/health/readiness reported, and the orphaned-VM warning present/absent.

## 6. Fix deployments stuck in `operation_in_progress` on Start/Stop failure

- [x] 6.1 `internal/deploy/deploymentControl.go`'s `Start` and `Stop` set `WorkflowStateOperationInProgress` before calling into the backend, but on failure returned the error directly without ever transitioning to a resolved state — unlike `Deploy`/`Destroy`, which already handle this correctly.
- [x] 6.2 Added `markOperationInterrupted` in `deploymentControl.go`, generalizing `destroy.go`'s existing `markDestroyInterrupted`: sets `WorkflowStateInterrupted{Error, InterruptedDuringOperation}` on failure, unregistering the signal handler first to avoid a race with it. Wired into both `Start` and `Stop`; refactored `markDestroyInterrupted` to delegate to the same shared helper.
