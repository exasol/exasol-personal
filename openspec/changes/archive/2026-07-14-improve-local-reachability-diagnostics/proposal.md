## Why

On macOS, the local deployment preset runs the database inside a VM and exposes it on `127.0.0.1:<port>` by forwarding traffic to the VM's guest network. When the process invoking `exasol` (a terminal, editor, or agent host) lacks macOS's "Local Network" permission, or is otherwise sandboxed away from it, the host-to-VM path breaks even though the VM and database are healthy. Today this surfaces as a generic database-startup timeout, an indefinite hang (the local backend's readiness wait has no timeout at all), or a raw driver/socket error — none of which point the user toward the actual cause. This sends users and agents investigating first-time evaluations toward the wrong areas (database startup, port conflicts, VM health) instead of the real one (macOS network permission for the invoking app).

## What Changes

- Apply the `waitTimeoutSeconds` deadline to the local backend's `Start` readiness wait, which today is silently ignored (`_ int`), and give the local backend's `Deploy` readiness wait (used by `exasol install local`, which has no timeout parameter in its interface at all today) the same bounded behavior — a blocked local deployment currently polls forever instead of ever reaching a failure.
- Classify local database-connection failures during install/start (readiness polling), `exasol connect`, and `exasol shell host`/`exasol shell container` as either "local runtime reachability problem" or the existing generic failure, instead of always treating non-auth errors as "not ready yet."
- Add a user-facing reachability error that explains macOS's Local Network permission applies to the invoking terminal/editor/agent (with examples: iTerm2, kitty, VS Code, a sandboxed agent host) even though the endpoint is `127.0.0.1`, and how to grant it.
- Add a distinct diagnosis step that checks reachability of every forwarded port (not just the one currently in use) so a network-wide problem (all ports affected) can be distinguished from a database-specific problem (only the database port affected) or a still-booting VM (transient).
- Add `exasol diag local`: a read-only command reporting local VM status, reported guest IP, bound host ports, per-port forwarder reachability, database readiness, and local platform/architecture support — usable at any time, not only on failure.
- Fix `exasol connect`'s database dial, which today receives no context/timeout at all and can hang indefinitely on any failure, local or otherwise.
- Fix `exasol start`/`exasol stop`: today, when the underlying backend call fails for any reason, the deployment is left permanently stuck reporting `operation_in_progress` — nothing ever transitions it to a resolved state, unlike `deploy`/`destroy`, which already do this correctly.

## Capabilities

### New Capabilities
- `local-reachability-diagnostics`: classifying local database-connection failures as reachability problems vs. generic failures, and the `exasol diag local` read-only diagnostic command.
- `deployment-lifecycle-recovery`: ensuring `start`/`stop` failures transition the deployment to a recoverable interrupted state instead of leaving it stuck reporting an in-progress operation.

### Modified Capabilities
- `exasol-local-deployment`: the "Start local deployment" scenario changes from "waits until database accepts connections" (unbounded) to a bounded wait that can report a reachability failure distinctly from a generic one; the "Connect to local database" scenario gains a bounded dial instead of an unbounded one.

## Impact

- `internal/deploy/local_reachability.go` (new): the reachability classifier and typed error.
- `internal/deploy/ready.go`, `internal/deploy/shared.go`, `internal/deploy/local_backend.go`, `internal/deploy/local_runtime.go` (readiness polling, timeout application, classification wiring for install/start).
- `internal/deploy/connect.go` (classification wiring for `exasol connect`); `internal/deploy/local_backend.go`'s `OpenHostShell`/`OpenCOSShell` (classification wiring for `exasol shell host`/`container`).
- `internal/deploy/deploymentControl.go`, `internal/deploy/destroy.go` (stuck-`operation_in_progress` fix, applied consistently to `Start`/`Stop`/`Destroy`).
- `internal/connect/connect.go`, `internal/connect/exasol/database.go` (bounded dial for `exasol connect`).
- `internal/localruntime` (health-check client consumed by the classifier and by `diag local`).
- `internal/deploy/diag_local.go`, `cmd/exasol/diagLocal.go` (new): the `exasol diag local` command surface.
- Depends on the local runner exposing forwarder reachability over its existing status socket as an external interface — see `design.md` for the exact contract assumed. This proposal does not depend on or describe the runner's internal implementation.
