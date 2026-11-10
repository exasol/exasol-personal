## Context

`auto` resolves to the first installed engine in the platform's order of preference, Podman then Docker, and to Podman when neither is installed, because Podman was the only engine the launcher could install. On Windows the Podman installation runs `winget install RedHat.Podman` behind a `[y/N]` approval. Docker Desktop is published in winget as `Docker.DockerDesktop`, installs machine-wide, and is not running after installation; its first start may show Docker's agreement in its own window.

## Decisions

1. **One prompt chooses and approves.**

   When `auto` finds neither engine on Windows, the prompt lists Podman first and Docker Desktop second, each with the winget command it runs. Choosing an engine approves its installation, and an empty or unrecognised answer cancels, as `[y/N]` defaults to no. A selection prompt followed by a separate install approval was rejected because the user would answer twice for one decision. A Docker that `auto` passes over because it runs Windows containers still counts as present, so the prompt never offers to install Docker Desktop over it and the deployment keeps the Podman path.

2. **The choice is saved before installing.**

   The chosen engine is written to the deployment's `container_engine` through the same manifest update as `exasol config set`. A run that stops after installing, for example because Docker Desktop is still starting, resumes on the chosen engine without asking again, and `exasol config get container_engine` shows the choice. Keeping `auto` was rejected because the outcome of a re-run would depend on which engines happen to be installed by then.

3. **The launcher starts only a Docker Desktop it installed in the same run.**

   After installing Docker Desktop, the launcher starts its executable from the machine-wide installation directory, detached so it outlives the command, and polls `docker version` until the daemon runs Linux containers, for up to 10 minutes, telling the user to accept Docker's agreement if Docker Desktop shows it. The `docker desktop start` command was rejected because it exists only in recent Docker Desktop releases. When the wait ends without a daemon, the command fails with directions to finish starting Docker Desktop and re-run. An installed Docker that is not running is still reported, not started, because a scripted run should not launch a desktop application the user set up themselves.

4. **Installation requests name the deployment's engine.**

   A deployment whose engine is missing on Windows states that the deployment uses that engine, because the saved choice or an explicit `container_engine` decides what is installed. The approval prompt and the failure without a terminal both name the `exasol config set --container-engine` command that selects the other engine, unless that engine is installed but cannot be used, such as a Docker that runs Windows containers, and the failure names the winget command to run by hand. A deployment that recorded its engine cannot change it, so neither the installation request nor Docker's readiness failures suggest the other engine for it.

5. **Unattended runs install the preferred engine.**

   `--auto-approve` installs Podman, the first engine in the Windows order of preference, so existing scripts keep their behaviour. Without a terminal and without `--auto-approve`, the command fails and names `--auto-approve` and `exasol config set --container-engine`, which works for `install` and `deploy` alike once the deployment is initialized, while the `--container-engine` flag exists only on `init` and `install`.

6. **Docker is found after installation in any terminal.**

   Windows fixes a process's `PATH` at start-up. Docker detection on Windows refreshes `PATH` from the registry, as Podman detection does, so commands run from terminals opened before the installation find Docker.

7. **A Podman answering for Docker is named.**

   A Podman machine serves Docker's API pipe on Windows and answers the `docker` command with its own version, which stops Docker Desktop from starting. The Docker readiness check reads the daemon's server components from `docker version` and recognises `Podman Engine`. The wait then fails at once, naming Podman and how to release the pipe, because Docker Desktop cannot start until Podman releases it; outside the wait the check names Podman instead of reporting its version as an outdated Docker Engine, and `auto` treats such a `docker` command as Podman's, as it treats Podman's Docker-compatible command. The pipe advice applies only when the `docker` command targets Docker's default pipe; when `DOCKER_HOST` or the current Docker context selects another endpoint, the failure names that endpoint and how to point the command at Docker Desktop instead, because stopping Podman would not help.

8. **Licensing is documented, not prompted.**

   Docker Desktop requires a paid subscription for larger organisations. The user documentation states this and links to Docker's terms; the prompt stays a single choice.

## Risks

Installing Docker Desktop requires administrator approval and can require a restart when Windows features such as WSL are missing. The 10-minute wait then ends with directions, and the saved choice lets the re-run continue on Docker.
