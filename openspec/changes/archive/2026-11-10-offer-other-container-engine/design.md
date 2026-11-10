## Context

`auto` picks the first installed engine and deliberately does not test whether it works, so a Docker daemon that is not running is reported rather than silently replaced. Preparation checks an engine's readiness before the deployment records its engine, and the record keeps a started deployment on its engine until it is destroyed. On Windows the launcher can install Podman and Docker Desktop through Windows Package Manager; on Linux it installs neither.

## Decisions

1. **Offer the alternative during preparation, before the engine is recorded.**

   The local backend knows whether `auto` chose the engine and owns the deployment configuration, so it handles a failed readiness check: it offers the other engine only for an `auto` choice on a deployment without a recorded engine. Explicit choices and started deployments therefore keep their engine without a separate check. Only a failure of the engine's own readiness check leads to an offer; a failure of the launcher's own files, such as creating the runtime directory or recording the engine, is returned unchanged, because it would follow the deployment to the other engine.

2. **Only a person can accept the offer.**

   A transient failure such as Docker Desktop still starting would otherwise move a scripted deployment to the other engine permanently, and on Windows install it. `--auto-approve` and runs without a terminal decline the offer, and the failure names the `exasol config set --container-engine` command that selects the other engine, unless that engine is installed and fails its own readiness check, because selecting it would only trade one failure for another. Treating `--auto-approve` as acceptance was rejected for that reason.

3. **One prompt covers switching and installing.**

   When the alternative is missing on Windows, the offer lists its winget command, and accepting the offer approves that installation, so the user answers once. An installed Docker Desktop is then started and awaited as for any Docker installation the launcher makes.

4. **Offer only what can run.**

   Docker is offered when its daemon already answers with Linux containers, or on Windows when it is not installed and winget can install it. Podman is offered when it is installed, or on Windows when winget can install it. On Linux, `auto` chooses Docker only when Podman is not installed, and the launcher installs neither, so the failure carries the directions alone.

5. **The accepted engine is saved.**

   Accepting writes the alternative to the deployment's `container_engine`, as the installation choice does, so a re-run continues on it even when the engine `auto` prefers is still installed.

6. **A Docker running Windows containers is skipped with a reason.**

   `auto` passes over such a Docker and uses Podman, and the run states why. Making it a failure that triggers the offer was rejected because runs with `--auto-approve` on such hosts would then fail instead of installing Podman.
