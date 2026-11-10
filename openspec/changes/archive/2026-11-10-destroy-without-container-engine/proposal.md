## Why

`exasol destroy` removes a local deployment's containers through its container engine before deleting the deployment's data. When the engine was uninstalled, the destroy fails at the first container, so the deployment's data, which takes most of its disk space, cannot be freed through the launcher.

Related issue: [SPOT-33896](https://exasol.atlassian.net/browse/SPOT-33896).

## What Changes

- In a terminal, `exasol destroy` for a local deployment whose container engine command is not installed asks whether to destroy the deployment without the engine.
- Accepting deletes the deployment's data and runtime files and returns the deployment to its initialized state. The prompt names the containers and network the engine may still hold and the commands that remove them if the engine is installed again.
- Declining, `--auto-approve`, and runs without a terminal fail as before, because the engine may still hold the deployment's containers.

## Capabilities

### Modified Capabilities

- `docker-host-runtime`: A deployment whose engine was uninstalled can be destroyed.

## Impact

The change affects the local backend's destroy and the `destroy` command's prompts. Engine-backed destroys and cloud deployments are unchanged.
