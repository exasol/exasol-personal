## Context

See proposal.md for motivation.

Status inspects sidecars on demand: it lists the owned containers, compares
each with its saved definition, and reports the running state, the endpoints,
and reconciliation. Nothing runs between launcher commands. The launcher talks
to Podman today and is meant to support other container runtimes later.

## Goals / Non-Goals

**Goals:**

- A definition states how its service shows that it is ready, in the
  Kubernetes shape the rest of the container subset uses.
- Status separates a sidecar that is starting from one that is serving and one
  that is failing.
- Enable returns once a sidecar is serving, or reports why it is not.

**Non-Goals:**

- Restarting a sidecar whose probe fails. The restart policy governs only
  process exit.
- Application metrics or log collection.

## Decisions

**A single readiness probe determines health.** `readinessProbe` answers
whether the service serves, which is the question `health` reports. In
Kubernetes, `livenessProbe` exists to restart a container, a behavior the
launcher keeps out of scope, so the container subset carries `readinessProbe`
alone and a saved `readinessProbe` keeps its Kubernetes meaning.

**The launcher runs the probe on demand.** Status and enable evaluate the
probe of a running sidecar when they inspect it. An `exec` probe runs its
command inside the container through the container runtime; `httpGet` and
`tcpSocket` probes connect to the loopback address where the probed port is
published, which is the address status reports. This needs only command
execution, a container start time, and port publication from a runtime, so the
behavior stays the same across Podman on Linux, the macOS virtual machine, the
Windows Podman machine, and future runtimes.

**Network probes target published ports.** Validation requires an `httpGet` or
`tcpSocket` port to name, by number or by name, a container port with a
positive `hostPort`. An `exec` probe reaches any port from inside the
container.

**Health is derived from the container's start time.** Before
`initialDelaySeconds` has elapsed since the container started, health is
`starting` and the probe does not run. A probe that succeeds makes health
`serving`. A probe that fails keeps health `starting` until
`initialDelaySeconds + periodSeconds × failureThreshold` has elapsed, and makes
it `failing` afterwards. `health` is empty when the sidecar is not running or
declares no probe. `healthError` carries the cause of the latest failure.

**Probe defaults follow Kubernetes.** `periodSeconds` defaults to 10,
`timeoutSeconds` to 1, `failureThreshold` to 3, `initialDelaySeconds` to 0,
the `httpGet` path to `/`, and its scheme to `HTTP`. An omitted or zero timing
field takes its default. An `httpGet` probe succeeds on a status from 200
through 399, does not follow redirects, skips certificate verification for
`HTTPS`, and does not use a proxy. An `exec` probe succeeds on exit status 0
and runs its command without environment expansion, as Kubernetes does.

**The probe stays out of the container fingerprint.** Reconciliation recreates
a container whose fingerprint changes. The probe does not change the container
itself, so the fingerprint omits it and an edited probe applies on the next
status.

**Enable waits while health is starting.** After applying, enable re-runs
status once per second while `health` is `starting`, bounded by the probe's
startup window measured from the first evaluation, and returns the last result.
`--no-wait` returns the first result. Text output adds a call to action when
the result is `starting` or `failing`. Enable exits successfully in both
cases, because the sidecar is enabled and running; JSON consumers branch on
`health`. Deploy and start do not wait.

**The MCP catalog entry declares readiness on `/health`.** The MCP server
answers `GET /health` once it serves. The probe uses `periodSeconds: 10` and
`failureThreshold: 6`, so enable waits up to one minute for the server to
start.

## Risks / Trade-offs

[The probe runs only when a command asks] → status reflects the moment it
runs, and no history is kept. `healthError` carries the latest failure, which
is the information a user needs to diagnose a failing sidecar.

[A sidecar that restarts repeatedly keeps resetting its start time] → it can
stay `starting` across status calls. Enable bounds its wait by wall-clock time
from its first evaluation, so it always returns.

[Enable holds the deployment lock while it waits] → other commands on the same
deployment wait up to the probe's startup window. `--no-wait` avoids that.
