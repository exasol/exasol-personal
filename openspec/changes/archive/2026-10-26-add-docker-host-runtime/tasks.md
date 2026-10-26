## 1. Select the Container Engine

- [x] 1.1 Add the `container_engine` local configuration variable on Linux and Windows, validated against each platform's list of engines, resolve it when the local runtime is selected for every workflow, and build the Nano installation and sidecar runtime from a `ContainerRuntime` chosen by the engine. Add unit tests for the option's platform gating, validation, display, and selection, and run `task fmt` and `task lint`. Commit as `feat: Select the container engine of local deployments`.

## 2. Run Containers through Docker

- [x] 2.1 Add the Docker `ContainerRuntime`, reusing the Podman implementation where the CLIs agree and overriding listing, existence, attachment, removal, image existence, network creation and inspection, replacement, entrypoint, and image-mount spelling. Express the UDF sandbox permission as a spec field each engine spells, add `RunToCompletion` for one-off containers, and keep the Podman argv unchanged. Add unit tests for the Docker argv and replies, and run `task fmt` and `task lint`. Commit as `feat: Run local containers through Docker`.

## 3. Prepare Hosts for Docker

- [x] 3.1 Add the Docker host preparer, which checks the CLI, the daemon, and Linux containers with guidance and no approval, reports daemon state as container-host state, and skips the in-VM flush on Windows, while Podman deployments keep their preparers. Make the data directory traversable and create the user staging directories for Docker deployments, empty root-owned data through a cleanup container when destroy hits a permission error, and name Docker commands in diagnostics and reachability guidance. Add unit tests, and run `task fmt` and `task lint`. Commit as `feat: Prepare Linux and Windows hosts for Docker`.
- [x] 3.2 Report a Docker daemon that does not answer as an unreachable engine that keeps the recorded state, add the engine's error output to a failed Nano start, and classify Docker Desktop's port conflicts. Add unit tests, and run `task fmt` and `task lint`. Commit as `fix: Keep Docker deployments in step with the Docker daemon`.
- [x] 3.3 Translate engine container states into an engine-neutral lifecycle that counts restarting and paused containers as running, let the Nano installation run the Docker data directory preparation once recovery has settled the directory, remove recovered data and the engine record last on destroy, treat only database data without a record as a Podman deployment, and find digest-pinned images on Docker. Add unit tests, and run `task fmt` and `task lint`. Commit as `fix: Make local container state and cleanup engine-neutral`.

## 4. Resolve `auto` and Record the Engine

- [x] 4.1 Default `container_engine` to `auto`, which selects the first installed engine in the platform's order of preference, Podman then Docker, where Docker must be real and run Linux containers, and Podman when neither is installed. Record the resolved engine at first preparation, treat runtime directories without a record as Podman, refuse to change a recorded engine with guidance to destroy, and report the engine in use from `diag local`. Pass the caller's context to detection. Add unit tests, and run `task fmt` and `task lint`. Commit as `feat: Resolve auto and record the container engine of local deployments`.
- [x] 4.2 Reject an empty `container_engine` value, and refuse an engine change for a running deployment with the guidance to destroy it, stating that destroying deletes its data. Add unit and launcher tests, and run `task fmt` and `task lint`. Commit as `fix: Explain container engine changes`.

## 5. Tests and Documentation

- [x] 5.1 Pin the engine in launcher tests that fake one, read the recorded engine in live tests that inspect containers, make the sidecar live tests independent of engine-specific defaults and listing formats, and add launcher tests for the option marked `openspec("docker-host-runtime")`. Commit as `test: Make local tests aware of the container engine`.
- [x] 5.2 Describe Docker in the local preset. Commit as `feat: Describe Docker in the local preset`.
- [x] 5.3 Document the container engine, Docker prerequisites and behaviour, and the custom SLC requirement, regenerate the CLI reference, and add the changelog entries. Commit as `docs: Document Docker local deployments`.

## 6. Verify

- [x] 6.1 Run the local deployment tests on Linux with rootless Docker, rootful Docker, and Podman.
- [ ] 6.2 Run the local deployment tests on Windows with Docker Desktop and with Podman only, and on macOS.
