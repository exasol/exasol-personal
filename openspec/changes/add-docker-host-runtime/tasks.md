## 1. Select the Container Engine

- [ ] 1.1 Add the `container_engine` local configuration variable with values `auto`, `docker`, and `podman` on Linux and Windows, defaulting to `auto`. Record the resolved engine in a launcher-owned marker in the direct-host runtime directory at first preparation, treat a runtime directory without a marker as Podman, and resolve the engine once in direct-host runtime selection so every local workflow uses the same answer. For `auto`, select Docker only when `docker` resolves and its server identifies as Docker rather than Podman. Reject an explicit `container_engine` in `config set` that differs from the recorded engine, with guidance to destroy first. Add unit tests for automatic selection with and without Docker, explicit selection of each engine, a `docker` command served by Podman, a Docker daemon serving only Windows containers, an existing deployment without a marker, a recorded or selected engine that is unavailable, the rejected and accepted reconfigurations, and reselection after destroy, then run `task all`. Commit as `feat: Select the container engine of local deployments`.

## 2. Run Nano through Docker

- [ ] 2.1 Give the installation policy a Docker command set while keeping the Podman argv unchanged: exact-name container existence through `ps`, image existence through `images --quiet`, remove-then-run instead of `--replace`, the Docker security options, read-only SLC image mounts, `Loaded image ID:` parsing, and engine-named diagnostics. Depend on the `LocalInstall` interface from the direct-host runtime. Add unit tests for the Docker argv and lifecycle, and assert the Podman argv is unchanged, then run `task all`. Commit as `feat: Run Nano through Docker`.

## 3. Prepare Linux Hosts for Docker

- [ ] 3.1 Add the Linux Docker preparer: check that the CLI resolves and the daemon answers with guidance on daemon state, socket permission, and CLI context, report daemon state as container-host state with logging, make the data directory `0755` at every start, and create `bucketfs/bfsdefault/default` and `jdbc` as the launcher user before Nano first starts. Empty root-owned data through a digest-pinned helper container when destroy hits a permission error. Name Docker commands in Linux diagnostics and reachability guidance for Docker deployments. Add unit tests for readiness, status, directory preparation, and the destroy fallback, and run `task all`. Commit as `feat: Support Docker for Linux local deployments`.

## 4. Prepare Windows Hosts for Docker

- [ ] 4.1 Add the Windows Docker preparer, which skips Podman installation and the Podman machine, uses the readiness and status checks from task 3, and applies the decided Nano startup durability behavior. Name Docker commands in Windows guidance for Docker deployments. Add unit tests for preparer selection, the absence of approval requests, and status, and run `task all`. Commit as `feat: Support Docker for Windows local deployments`.

## 5. Document and Verify

- [ ] 5.1 Make the launcher and live tests engine-aware: tests that fake an engine pin it with `--container-engine` and fake that engine, including the bind-conflict and legacy-port reconfiguration tests and the unresponsive-engine status fixtures, and live assertions query the recorded engine, including the leftover-container check and the Virtual Schema sidecar. Mark Docker coverage with `openspec("docker-host-runtime")`. Document Docker preference, the `--container-engine` option, engine stickiness, rootful and rootless Docker, the `docker` group's privilege, and the custom SLC skeleton requirement, add the changelog entry with a `--container-engine` example, and run `task all`. Commit as `docs: Document Docker local deployments`.
- [ ] 5.2 Run the local deployment tests on Linux with rootful Docker, rootless Docker, and Podman only, on Windows with Docker Desktop and with Podman only, and on macOS.
