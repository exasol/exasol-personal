## Context

See `proposal.md` for motivation. The existing `PodmanInstall` implements the required persistent Nano, SLC, recovery, migration, and diagnostic behavior but assumes commands and filesystem paths belong to the host. The macOS runtime invokes an older runner that owns the database container and exposes SSH details in state. The verified v2 runner instead provides a Podman-enabled VM, a VirtioFS share at `/mnt/host`, labeled port forwarding, and `launcher run`.

The Linux runtime must remain a direct host-Podman implementation. Runner release resources and their pinned production version are intentionally unchanged, so development integration uses the existing runner override.

## Goals / Non-Goals

**Goals:**
- Reuse one Podman installation implementation on Linux and in the macOS VM.
- Make commands, filesystem operations, and artifact paths explicit properties of an execution environment.
- Keep SSH and guest addressing private to the VM runner.
- Preserve existing local data, SLC, recovery, and diagnostic semantics.
- Keep service-forward configuration immutable for one VM start.

**Non-Goals:**
- Releasing or pinning the v2 runner.
- Adding a dynamic forwarding API to a running VM.
- Generalizing the VM launcher into a container orchestrator.
- Changing cloud deployment behavior or adding Linux local shell support.

## Decisions

### Use an execution environment for commands and filesystem operations

`PodmanInstall` will receive an environment that executes commands and performs the filesystem operations needed by installation. The Linux implementation delegates to `os` and `exec`; the VM implementation delegates to `launcher run` and POSIX guest operations.

This keeps recovery and migration decisions in one installation implementation. Passing only a command runner was rejected because direct `os.Stat`, `os.Rename`, directory reads, atomic report writes, and cleanup would still target the macOS host.

### Represent artifacts with paired paths

Materialized artifacts will carry `HostPath` and `RuntimePath`. Linux uses an identity mapping. macOS stages artifacts below the runner's shared directory and maps the relative suffix below `/mnt/host`.

The embedded Nano image is materialized atomically to a stable shared runtime-artifact path before installation. Custom SLC packages use the same staging and mapping rule. Guest-only paths such as persistent database data have only a runtime path.

Installation will always invoke `podman load -i <runtime-path>`. Streaming the image on stdin was rejected because `-i` works for both direct and transported Podman and avoids special transport behavior in installation.

### Separate declared service ports from effective host endpoints

Installation configuration declares the fixed guest service ports before any container is started. The macOS runtime uses those declarations to start the VM with labeled forwards, reads the effective host ports from runner state, and then starts the Podman installation. Podman publishes container ports on the guest using the declared guest ports; deployment metadata uses the effective host ports.

This retains immutable VM forwarding. A dynamic add-forward API was rejected because it adds synchronization and state mutation solely to avoid a sequencing step that can be resolved from static installation metadata.

### Treat the runner as an execution boundary

The macOS runtime calls runner `run` for noninteractive commands and `run --tty` for shells. It does not parse SSH keys, SSH ports, or guest IPs. Host shell is runner `run` without a command. Because Nano is shell-less, container shell mounts the deployment container rootfs and uses the VM shell in the container's namespaces.

The runtime rejects runner major versions below v2 before initialization or startup. Development versions remain available through the existing forced reconciliation and override mechanisms. Production resource publication is deferred.

### Order lifecycle around environment availability

Start brings up the VM environment before installation and stops on any installation failure while retaining diagnostics. Stop removes the disposable container before stopping the VM. Status requires both a running VM and the exact deployment container. Destroy cleans up while the VM is available when necessary, then removes VM-owned host state.

Existing Linux ordering remains direct because its environment is always available.

### Preserve legacy guest data through shared migration logic

The VM data disk remains mounted at `/var`, with Podman storage and deployment data in guest persistent storage. The shared installation's existing legacy overlay migration handles an older runner-created `exasol-local-db` container before recreating it with a persistent `/exa` mount. Host-shared legacy payloads are disposable and removed during preparation.

## Risks / Trade-offs

- **Remote filesystem operations add command round trips** -> Keep operations coarse, avoid polling through the environment, and retain local direct operations on Linux.
- **Guest command quoting could corrupt paths** -> Pass argv through runner `run`, stage under controlled paths, and test spaces and special characters at the runner boundary.
- **A VM can be running while installation fails** -> Return the original installation error, collect Podman diagnostics through the environment, and leave explicit stop/destroy recovery available.
- **Old runners can still own containers** -> Enforce the v2 contract before invoking start and keep production pin changes out of this branch until release preparation.
- **Forward configuration precedes effective endpoint discovery** -> Use fixed guest service declarations and labels; only host port zero remains dynamic and is resolved from runner state.

## Migration Plan

1. Introduce execution environments and paired artifact paths while retaining Linux identity behavior.
2. Verify Linux Podman unit tests and a Linux local smoke deployment.
3. Switch the macOS runtime to runner v2 lifecycle, labeled forwards, and guest installation.
4. Exercise an existing VM data disk to verify legacy-container migration and persistence.
5. Use the runner override for Mac end-to-end testing until a separate release change publishes and pins v2.

Rollback before runner publication is code-only: revert the exasol-personal integration and continue using the existing pinned runner. Migrated persistent `/exa` data remains usable because migration is non-destructive and the old container is removed only after successful installation.
