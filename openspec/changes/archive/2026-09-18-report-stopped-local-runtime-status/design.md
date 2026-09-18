## Context

`GetStatus` resolves a deployment recorded as running by asking the local runtime whether it
is running, and only then probing the database. That first question is asked through
`localVMStoppedStatus`, which treats any error from the runtime as "no answer" and falls
through to the database probe.

On Linux the question always gets an answer: `podman container exists` exits 1 for an absent
container, so the runtime reports not-running and the status resolves to `stopped`. On
Windows the same command cannot reach a stopped machine and fails with a connection error,
so the runtime returns an error and the fall-through produces `database_connection_failed`.

The launcher already knows how to make a Windows runtime answerable — `EnsureQueryable`
starts the machine — and `probeLocalRuntimeLiveness` uses it on the start and stop paths.
Status cannot: `exasol status` applies a five-second timeout by default
(`deployment-status-timeout`), a WSL2 machine takes considerably longer to boot, and starting
a machine is a side effect users do not expect from inspecting a deployment.

## Goals / Non-Goals

**Goals:**

- Report the same status on every platform when the deployment's container is not running.
- Keep `exasol status` free of side effects on the host.
- Keep the distinction between "not running" and "cannot tell" intact on the paths that rely
  on it.

**Non-Goals:**

- Start, create, or otherwise manage the Podman machine from `exasol status`.
- Correct the persisted workflow state from `exasol status`, which holds a shared lock;
  `reconcileLocalVMState` already does that under the exclusive lock on start and stop.
- Change how macOS resolves status. Its runner is host-side, so it has no environment layer
  that can be stopped independently of the deployment.

## Decisions

1. **Ask the container host, not only the container.** The direct-host runtime already
   isolates platform differences behind its preparer. Extend that seam with a read-only
   question about whether whatever hosts the containers is running. Windows answers from the
   Podman machine's state; Linux answers yes unconditionally, because containers run on the
   host kernel and there is nothing separate to be stopped.

   The alternative — teaching the deployment workflow to recognize Windows and inspect the
   machine itself — would put platform knowledge back into that package, which
   `local-runtime-boundaries` keeps out of it.

2. **Ask only when the container probe could not answer.** `HostRuntime.Status` keeps probing
   the container first and consults the container host on failure, because a stopped host
   settles what the probe could not: nothing runs inside one. A healthy deployment therefore
   costs no extra call, and the existing status resolution needs no change — it receives a
   clean answer where it previously received an error.

3. **Leave the start and stop paths alone.** `probeLocalRuntimeLiveness` keeps calling
   `EnsureQueryable` and keeps its three-valued answer. Those paths hold the exclusive lock,
   intend to change the machine's state anyway, and are not bound by the status timeout.
   Decision 1 makes their probe cheaper in the stopped case but does not change its contract.

4. **Give `database_connection_failed` a message.** It is the only status the launcher
   produces without one, and `formatStatusText` prints `Error` nowhere, so the text output
   was empty beyond the status word. The message covers the residual case this change does
   not convert to `stopped`: the environment is running but the database is unreachable.

## Risks / Trade-offs

Reading the machine state adds one `podman machine` invocation, and only where the container
probe already failed — a case that previously produced no usable answer at all. A machine that
is running while the deployment's container is not still resolves through the container probe,
unchanged, and costs nothing extra.
