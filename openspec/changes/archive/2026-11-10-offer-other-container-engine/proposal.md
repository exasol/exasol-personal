## Why

When the container engine `auto` chose for a new local deployment is installed but cannot be used, for example because the Docker daemon is not running or the Podman machine cannot start, the deployment fails even when the other engine would work. Offering the other engine keeps a broken or restricted installation from stopping a user who is evaluating Exasol Personal.

Related issue: [SPOT-33478](https://exasol.atlassian.net/browse/SPOT-33478).

## What Changes

- When the engine `auto` chose fails its readiness check for a deployment that has not started, an interactive run states why and offers the other engine, and deploys on it when the user accepts.
- On Windows, the offer includes installing the other engine through Windows Package Manager when it is missing.
- The accepted engine is saved as the deployment's `container_engine`.
- Explicitly selected engines, deployments that have started, and runs with `--auto-approve` or without a terminal keep their engine; they fail with the cause, how to resolve it, and the `exasol config set --container-engine` command that selects the other engine.
- When `auto` skips a Docker that runs Windows containers, the run states why it uses Podman.

## Capabilities

### Modified Capabilities

- `docker-host-runtime`: An unusable automatic engine choice can be replaced before the first start.

## Impact

The change affects local runtime preparation and the command-layer prompt. Scripts keep their engine: the offer needs a person to accept it.
