## 1. Offer the Other Engine

- [x] 1.1 When preparation of an engine `auto` chose fails for a deployment without a recorded engine, have the local backend find the other engine that can run here (Docker answering with Linux containers, Podman installed, or on Windows either engine installable through winget), offer it through the preparation options with the cause and any winget command, and on acceptance save it as `container_engine` and prepare it with its installation approved. Decline the offer under `--auto-approve` and without a terminal, and append the `exasol config set --container-engine` command to the failure. Report why `auto` skipped a Docker that runs Windows containers. Add unit tests for the offer, declining, unattended runs, explicit and recorded engines, the saved engine, and installation on Windows, and launcher tests marked `openspec("docker-host-runtime")` for the unattended failure. Document the offer in the local deployment and troubleshooting pages, and add the changelog entry. Run `task all`. Commit as `feat: Offer the other container engine when the automatic choice cannot be used - SPOT-33478`.

## 2. Validation

- [ ] 2.1 On Windows, check the offer with Docker Desktop stopped and Podman missing, and with a Podman machine that cannot start and Docker missing, accepting and declining each; on Linux, check the failure directions with an unusable Docker.

## 3. Archive

- [x] 3.1 Archive the change into `openspec/specs`. Commit as `docs(openspec): Archive offer other container engine change`.
