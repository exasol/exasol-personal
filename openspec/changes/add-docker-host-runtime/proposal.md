## Why

Local deployments on Linux and Windows require Podman, which many developers do not have while Docker is already installed and running on their machines. On Windows the launcher installs Podman and a WSL2 Podman machine on first use, a heavyweight host change that some organizations restrict. Using a Docker installation the user already has removes that setup for most users while keeping the Podman path for everyone else.

## What Changes

- Run the local database through host Docker on Linux AMD64, Linux ARM64, and Windows AMD64 when Docker is installed, and fall back to the existing Podman path when it is not.
- Add a `container_engine` option to local deployments on Linux and Windows, set with `--container-engine auto|docker|podman` on `init local`, `install local`, and `config set`. `auto`, the default, applies the preference above; an explicit choice is used without fallback. For example, `exasol init local --container-engine podman` keeps Podman on a host that also has Docker.
- Use Docker only as found: the launcher never installs, configures, or starts Docker, and reports why an installed Docker cannot be used.
- Keep each deployment on the container engine it was created with, so installing or removing Docker later does not orphan an existing deployment, and reject engine changes until the deployment is destroyed. Deployments created before this change keep Podman.
- Keep the user-facing parts of the deployment directory usable without elevated rights when Docker runs containers as root: staging BucketFS content and JDBC drivers, and destroying the deployment.
- Report a stopped Docker daemon as a stopped deployment.

macOS keeps its VM-backed Podman runtime.

## Capabilities

### New Capabilities

- `docker-host-runtime`: Select host Docker for direct-host local deployments automatically or by explicit choice, run Nano and its UDFs through it, and keep the deployment directory manageable by the launcher user.

### Modified Capabilities

- `exasol-local-deployment`: Provide the Linux and Windows local deployment through host Docker when available, and report a stopped Docker daemon as a stopped deployment.

## Impact

The change affects direct-host runtime selection, the local configuration options of the `init`, `install`, and `config` commands, the host environment preparers, the Nano installation policy and its command construction, local diagnostics and reachability guidance, deployment state, unit and live tests, user documentation, and release notes.

Fresh Linux and Windows deployments on hosts with Docker run under Docker instead of Podman, and a Windows host with Docker is no longer asked to install Podman. Existing deployments are unaffected. Under Docker, custom SLCs must ship the root directory skeleton the UDF sandbox mounts into, because Docker cannot give an image mount the writable overlay Podman provides.
