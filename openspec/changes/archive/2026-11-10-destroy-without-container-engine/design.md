## Context

A direct-host local deployment keeps its database data in its deployment directory and its containers and network in its container engine. `exasol destroy` removes the containers and network first, so a deployment whose engine command is gone cannot be destroyed. `exasol remove` deletes the whole deployment directory, including its configuration, without destroying anything.

## Decisions

1. **Only a missing command offers the engine-free destroy.**

   The offer appears when the deployment's engine command does not resolve, after the same `PATH` refresh deployment uses on Windows. An engine whose command resolves but whose daemon or machine does not answer still fails as before, because it usually answers again and still holds the deployment's containers.

2. **Only a person can accept.**

   Uninstalling an engine does not always remove what it holds; a Podman machine, for example, can outlive the Podman installation. Destroying without the engine can leave those containers behind, so `--auto-approve` and runs without a terminal fail as before, using the shared approval modes. Declining runs the engine-backed destroy, which fails as before.

3. **The prompt names what may stay behind.**

   The deployment's service containers carry its container's name as a prefix, and its network is named after it, so the prompt shows one `rm -f` command filtered by that name and one `network rm` command for the engine.

4. **The deployment returns to its initialized state.**

   An accepted destroy deletes the deployment's data, its runtime directory including the recorded engine, and the files written beside it, then completes like any destroy, so the deployment can be deployed again on any engine. `exasol remove` stays the way to delete the directory itself.

## Risks

Data files that only a container could delete, such as root-owned files under rootful Docker, cannot be removed without the engine. The destroy then fails, names the directory, and tells the user to delete it with administrator rights and re-run the command.
