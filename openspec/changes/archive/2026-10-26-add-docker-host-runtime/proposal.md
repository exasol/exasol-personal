## Why

Local deployments on Linux and Windows required Podman, and on Windows the launcher installed Podman and a WSL2 machine on first use, a host change many organizations restrict. Docker is often already installed. Supporting both engines lets users run Exasol Personal on the container runtime they have.

Related issue: [SPOT-33475](https://exasol.atlassian.net/browse/SPOT-33475).

## What Changes

- Run Linux and Windows local deployments on Docker or Podman, chosen per deployment with `--container-engine auto|docker|podman` on `init local`, `install local`, and `config set`.
- Resolve `auto` to the first installed engine in a per-platform order of preference, Podman then Docker, and to Podman when neither is installed, because Podman is the engine the launcher can install.
- Use Docker as the user configured it, including the active context, and explain why it cannot be used instead of installing, starting, or replacing it.
- Keep each deployment on the engine it first started on until it is destroyed, and treat deployments created before engine selection as Podman deployments.
- Give UDFs the sandbox permissions Docker requires, and keep file staging and destroy available to the launcher user when Docker runs containers as root.
- Let Docker restart a running deployment with its daemon once it has been stopped and started again after installation, including its sidecars, which now default to the `Always` restart policy.
- Report a Docker daemon that does not answer as an unreachable engine, keeping the deployment's recorded state.

macOS keeps its VM-backed Podman runtime.

## Capabilities

### New Capabilities

- `docker-host-runtime`: Engine selection, Docker use as found, restarts with the Docker daemon, UDFs under Docker, and a deployment directory the launcher user can manage.

### Modified Capabilities

- `exasol-local-deployment`: Install, Linux status, and startup durability in terms of the deployment's container engine.
- `linux-host-podman-install`: The direct-host Nano lifecycle in terms of the deployment's container engine.
- `sidecar-configuration`, `sidecar-lifecycle`: Sidecars default to the `Always` restart policy.

## Impact

New deployments on hosts with Docker and without Podman run on Docker; other hosts and existing deployments are unaffected. Docker deployments run with weaker container isolation, restart with the Docker daemon, and need custom SLCs that ship the directories the UDF sandbox mounts into.
