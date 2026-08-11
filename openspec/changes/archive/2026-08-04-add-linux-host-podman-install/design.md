## Context

`localruntime.Runtime.Start` currently accepts VM sizing and a raw port string, while `LinuxHostRuntime` ignores the input and delegates to an incomplete `PodmanInstall`. The Nano artifact is an OCI archive whose runnable reference must be recovered from `podman load`, and deployment state must persist outside disposable containers.

## Goals / Non-Goals

**Goals:**

- Separate common runtime configuration from VM-only sizing without changing the existing runtime method signature.
- Give the runtime-to-install boundary a typed, runtime-neutral container configuration.
- Port only the persistent core of the reference container lifecycle with deterministic errors and unit-testable command construction.

**Non-Goals:**

- Apply CPU, memory, or data-size quotas to host containers.
- Implement image caching or image cleanup.
- Implement migration, incomplete-create recovery, SLCs, version checks, log followers, diagnostics, readiness reporting, or non-Linux runtimes.
- Add user-facing configuration fields.

## Decisions

1. Embed common configuration in `VMConfig`.

   `RuntimeConfig` owns the raw port setting and `VMConfig` embeds it while retaining CPU, memory, and data size. The runtime interface continues accepting `VMConfig` to keep this preparatory change small; Linux reads only the embedded common settings. A split start interface was rejected because Go embedding does not make `VMConfig` assignable to `RuntimeConfig` and runtime dispatch would broaden the refactor.

2. Pass resolved container settings to the installer.

   `localinstall.StartConfig` carries the host and container DB ports, data directory, Podman options, and Nano init parameters. `LinuxHostRuntime` owns translation from launcher runtime configuration and deployment paths; `PodmanInstall` owns validation and command execution. Hard-coding settings inside the installer was rejected because it would block later user-configuration wiring.

3. Keep Nano's internal port fixed.

   The reference runtime fixes Nano's database port at `8563`; the launcher's `db:<port>` value selects the host endpoint. Linux therefore publishes `<host-port>:8563`. Well-formed non-DB services are ignored because this runtime exposes no VM SSH or UI forwarding, while malformed entries and invalid DB ports fail before container startup.

4. Use disposable containers with persistent deployment data.

   The installer mounts `<runtime>/exa` at `/exa`, uses `--replace`, and does not use `--rm`. Stop force-removes the container with `--ignore`; destroy additionally removes the runtime directory. The presence of `/exa/exasol.conf` distinguishes an existing database from a fresh one for first-start parameters.

5. Derive a deterministic local image tag after every required load.

   Startup captures and forwards `podman load` output, parses the reported loaded image reference, and tags it as `localhost/<container-name>:latest`. Startup checks for an already-running exact container name first so it can return without resolving or loading the artifact. A hard-coded source tag was rejected because the artifact manager does not preserve that metadata as a separate API.

## Risks / Trade-offs

- Parsing human-readable `podman load` output depends on Podman's documented output contract -> isolate parsing, require an unambiguous reference, and cover accepted and rejected output with tests.
- The shared runtime method still accepts `VMConfig` for host runtimes -> isolate VM-only fields through embedding now and defer a broader runtime interface redesign.
- Re-loading occurs whenever the named container is not running -> accepted for this minimal slice; no caching behavior is planned here.
- Host Podman is unavailable in CI -> exercise commands through a fake executable and retain a Linux manual smoke test.
