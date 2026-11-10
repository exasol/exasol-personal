## 1. Install Docker or Podman on Windows

- [x] 1.1 Add the Docker Desktop winget install command and generalise the registry `PATH` refresh to Docker, and make Windows Docker detection use it. Add a container engine choice to the preparation options, presented by the command layer as one prompt listing Podman then Docker Desktop with their winget commands, answered with Podman under `--auto-approve`, and failing without a terminal with a message naming `--auto-approve` and `exasol config set --container-engine`. When `auto` finds neither engine on Windows, save the choice as `container_engine` through the manifest update `config set` uses, and prepare the chosen engine with its installation approved. Install Docker Desktop for a `docker` deployment after approval when it is missing, start it, and wait up to 10 minutes for Linux containers, failing with directions to finish starting it and re-run. Add unit tests for the prompt, the unattended answers, the saved choice, the Docker install and wait, and the `PATH` refresh, and launcher tests marked `openspec("docker-host-runtime")` where a fake winget allows. Document the choice, the Docker Desktop subscription terms, and the wait in the local deployment, system requirements, and troubleshooting pages, and add the changelog entry. Run `task all`. Commit as `feat: Choose Docker or Podman when installing a container engine on Windows - SPOT-33479`.

## 2. Validation

- [ ] 2.1 On Windows without Podman or Docker, check the prompt with each choice, a cancelled prompt, `--container-engine docker`, `--auto-approve`, and a run without a terminal, and a re-run from a terminal opened before the installation.

## 3. Archive

- [x] 3.1 Archive the change into `openspec/specs`. Commit as `docs(openspec): Archive Windows container engine installation change`.
