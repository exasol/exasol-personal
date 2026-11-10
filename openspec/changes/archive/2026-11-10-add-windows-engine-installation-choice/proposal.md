## Why

On Windows hosts without a container engine, the launcher could only install Podman. Organisations that allow Docker but not Podman had to install Docker Desktop themselves before they could evaluate Exasol Personal.

Related issue: [SPOT-33479](https://exasol.atlassian.net/browse/SPOT-33479).

## What Changes

- On Windows with neither engine installed and `container_engine` set to `auto`, an interactive run asks in one prompt whether to install Podman or Docker Desktop through Windows Package Manager.
- A deployment whose `container_engine` is `docker` installs Docker Desktop after approval, without asking which engine.
- After installing Docker Desktop, the launcher starts it and waits up to 10 minutes for it to run Linux containers before deploying.
- The engine chosen in the prompt is saved as the deployment's `container_engine`.
- Runs with `--auto-approve` install Podman, the platform's preferred engine; runs without a terminal and without `--auto-approve` fail and name `--auto-approve` and `exasol config set --container-engine`.
- The user documentation notes the Docker Desktop subscription terms.

## Capabilities

### Modified Capabilities

- `docker-host-runtime`: Windows hosts without an engine install the one the user chooses.

## Impact

The change affects Windows local runtime preparation, the command-layer prompt, and the winget wrapper. Linux and macOS are unaffected.
