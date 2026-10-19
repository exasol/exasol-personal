## Context

Local deployments on Linux and Windows run Nano through one direct-host runtime that drives host Podman. macOS runs the same installation policy against Podman inside its managed VM. The installation policy issues Podman CLI commands, several of which have no Docker equivalent, and it assumes the rootless Podman ownership model in which every file Nano writes into the host-visible `/exa` directory belongs to the launcher user.

The findings below were verified on Linux (WSL2, Ubuntu 24.04) against Docker Engine 29.9 in both rootful and rootless mode, using the full local live suite, an official Python SLC, the PostgreSQL Virtual Schema test, and stop, start, and destroy cycles. Docker Desktop on Windows has not been run.

## Goals / Non-Goals

**Goals:**

- Prefer an installed Docker on Linux and Windows without changing the Podman path for hosts that lack it.
- Let users choose the engine explicitly when the preference is wrong for them.
- Keep a deployment bound to one container engine for its whole life.
- Keep the deployment directory readable, writable where documentation directs users to write, and removable by the launcher user under rootful Docker.
- Keep the Podman command line unchanged, so Podman deployments, including macOS, behave exactly as before.

**Non-Goals:**

- Install, configure, start, or upgrade Docker.
- Run macOS deployments through Docker.
- Migrate an existing deployment from one engine to another.
- Support Docker on Windows ARM64 or any other unsupported local platform.

## Extension Points

The local runtime already separates platform, environment, and engine concerns. The change is expected to plug into these seams rather than add a parallel runtime.

1. **Runtime selection**: `newLocalRuntimeForPlatform` in the deploy package maps the host platform to a `localruntime.Runtime`. Every local workflow (backend creation, status reconciliation, SLC restarts, diagnostics) goes through it, so engine selection made here or below it is applied consistently.
2. **The `localruntime.Runtime` interface**: the complete lifecycle and durability contract (`Prepare`, `Start`, `Stop`, `Status`, `EnsureQueryable`, `Destroy`, the durability workaround, endpoints, health, shells). `HostRuntime` implements it for Linux and Windows and `MacVMRuntime` for macOS. Docker does not need a new implementation of this interface.
3. **The host environment preparer**: `hostRuntimeEnvironmentPreparer`, injected into `HostRuntime`, supplies everything that differs per direct-host platform: `EnsureReady` (approval-gated prerequisites), `EnsureStartable` (approval-free re-checks), `ContainerHostRunning` (status of whatever hosts the containers, read without starting it), `NewExecutionEnvironment`, and `Platform`. A Docker preparer per platform, or engine-aware preparers, fit here. Windows Podman installation and the Podman machine live entirely behind this seam, so selecting a Docker preparer skips them without touching shared code.
4. **The installation policy**: `localinstall.LocalInstall` (`Start`, `Stop`, `Status`, `Destroy`) is implemented by `PodmanInstall`, which owns the Nano container lifecycle, SLC materialization, bootstrap password handling, recovery, and migration. `HostRuntime` constructs the concrete `PodmanInstall` rather than depending on the interface. This is where Docker's command differences belong, either as an engine dimension of the existing policy or as a sibling implementation.
5. **The execution environment**: `localinstall.ExecutionEnvironment` abstracts command transport and installation-owned filesystem operations. The direct environment runs commands on the host; the Windows environment overrides `Run` and `Sync`; the macOS environment executes through the VM runner. Per-engine `Sync` behavior on Windows belongs here.
6. **Host-change approval**: `HostChangeRequest`, `HostChangeKind`, and the command-layer approver gate host-mutating preparation. The Docker path never mutates the host, so it raises no request; the Podman fallback keeps its existing approval flow.
7. **Platform guidance**: `HostRuntime.Platform()` selects Linux or Windows advice in local diagnostics and reachability messages. Those messages name engine-specific commands, so they need the engine as well as the platform.
8. **Local configuration variables**: the local backend's configuration variable definitions declare per-platform options, as `ports` is offered everywhere and VM sizing only on macOS. The CLI derives `--<name>` flags for `init`, `install`, and `config set` from them, and validation, defaults, and `config reset` follow from the same definition. The engine choice is one more platform-gated variable.
9. **Launcher-owned runtime markers**: the macOS runtime records the runner version in a launcher-owned marker beside its runtime state, separate from files whose schema an external tool dictates. An engine marker in the direct-host runtime directory follows the same pattern and is removed with the runtime directory on destroy.

## Decisions

1. **Select Docker when its CLI resolves, at first preparation.**

   With `container_engine` set to `auto`, a fresh direct-host deployment uses Docker when `docker` resolves on PATH and Podman otherwise. Presence of the CLI, not daemon reachability, decides, because a reachability-based rule would silently fall back to Podman (and on Windows, propose installing it) whenever Docker Desktop is not running or the user lacks socket permission. A failing Docker is reported with guidance instead (decision 4).

   The resolved `docker` must be identified as Docker rather than assumed. Some distributions install a `docker` command that runs Podman (`podman-docker`), which `docker --version` reveals without a daemon. A Docker daemon must also run Linux containers: GitHub's Windows runners ship a Docker Engine that runs only Windows containers, and Docker Desktop can be switched to that mode. `docker info` reports the daemon's `OSType`; a daemon reporting `windows` cannot run Nano, so `auto` treats that Docker as absent and an explicit `docker` fails with guidance to switch to Linux containers. A daemon that does not answer cannot be classified and is reported by decision 4.

2. **Let users choose the engine explicitly.**

   `container_engine` accepts `auto`, `docker`, and `podman` on Linux and Windows and defaults to `auto`. An explicit value is used as given: `docker` fails with the readiness guidance when Docker cannot be used rather than falling back, and `podman` follows the Podman path, including the Windows installation approval, even on a host with Docker. Expressing the choice as a configuration variable rather than a dedicated flag makes it available to `init local`, `install local`, and `config set` with the existing validation, display, and reset behavior. macOS does not offer it, because its VM ships Podman.

3. **Record the engine per deployment and never re-select it.**

   The resolved engine is recorded in a launcher-owned marker in the deployment's runtime directory at first preparation, and every later command uses it. A deployment whose runtime directory exists without a marker is a Podman deployment, which covers every deployment created before this change. Destroy removes the runtime directory and with it the marker, so a destroyed deployment selects again on its next deploy. Re-selecting on every command was rejected: installing Docker later would make the launcher look for the container in the wrong engine, report the deployment as stopped, and start a second Nano container against the same data directory while the first still holds the port.

   A recorded engine that is no longer available fails with guidance naming that engine; it does not fall back.

   `config set` rejects an explicit `container_engine` that differs from the recorded engine and directs the user to destroy the deployment first. `auto` is always accepted, because it never re-selects a recorded engine. Allowing the change for a stopped deployment was considered, since stop removes the container and the data directory is engine-neutral, but a rootful Docker deployment leaves root-owned data that a rootless Podman container cannot read, and SLC images would have to be fetched again into the other engine.

4. **Use Docker as found.**

   The Docker preparer checks only that the CLI resolves and the daemon answers. It requests no host-change approval and never installs or starts Docker. Failures name the likely causes seen in practice: daemon not running, socket permission (the user is not in the `docker` group or the session predates the group change), or a CLI context pointing at another daemon. The launcher honors the user's Docker context and `DOCKER_HOST` by invoking the CLI unchanged.

5. **Keep the Podman command line unchanged.**

   Engine differences are confined to the Docker path, so the Podman argv remains byte-for-byte identical and Podman deployments, including macOS, need no re-verification. Commands that differ are listed in the next section.

6. **Report daemon state as container-host state.**

   `ContainerHostRunning` answers from `docker info` on both platforms. On Linux this differs from Podman, which has no daemon and always reports running. A failing `docker info` is reported as not running and logged with its error and output, so a status of `stopped` caused by permissions or a wrong context is diagnosable.

## Running Nano with Docker

| Podman argument or command | Docker replacement | Effect compared with Podman |
|---|---|---|
| `run -d --replace --name <n>` | `rm --force <n>` when the container exists, then `run -d --name <n>` | Same outcome. Not atomic, but `Start` has already returned when the container runs, so only a stopped container is replaced. |
| `--security-opt unmask=ALL` | none required | `systempaths=unconfined` was tested and is not needed for UDFs under rootful or rootless Docker. |
| (Podman's default seccomp profile) | `--security-opt seccomp=unconfined` | Required: Docker's default profile blocks the namespace creation the UDF sandbox performs, and UDFs fail with `VM crashed`. This is broader than Podman's default; see Risks. |
| (no AppArmor under rootless Podman) | `--security-opt apparmor=unconfined` | Not verifiable on the test host, which has no AppArmor. Hosts with AppArmor apply `docker-default`, which restricts mounts. Keep until verified otherwise. |
| `--security-opt label=disable` | unchanged | Accepted by Docker with or without SELinux. |
| `--shm-size=512mb`, `--pids-limit=-1`, `-p 127.0.0.1:<port>:8563` | unchanged | The loopback binding is kept for exposure reasons; the WSL2 IPv6 relay problem it also solves belongs to the Podman machine. |
| `-v <data>:/exa:Z`, bootstrap `-v <dir>:/run/secrets:ro,Z` | unchanged | Same behavior. |
| `--restart always` / `no` | unchanged | Docker's daemon restarts running containers after a reboot; rootless Podman needs `podman-restart.service` for that. A running deployment therefore comes back after reboot under Docker. |
| `--mount type=image,source=<slc>,destination=<target>,rw=true` | same without `rw=true` | Docker rejects `rw`, and its image mounts are always read-only (`readonly=false` is ignored). See the UDF section. |
| `container exists <n>` | `ps --all --filter name=<n> --format {{.Names}}`, matching the exact name | `docker container inspect` exits 1 both for a missing container and an unreachable daemon, so it cannot distinguish them. The name filter is a substring match, hence the exact comparison. |
| `image exists <ref>` | `images --quiet <ref>`, non-empty output | Same outcome. |
| `rm --force --ignore <n>` | existence check, then `rm --force <n>` | Same outcome. |
| `load -i <archive>` | unchanged | Docker loads the OCI archive and prints `Loaded image: <ref>`. It prints `Loaded image ID: <id>` for an untagged image, so both forms are parsed. |
| `import --change LABEL…`, `images --filter label=… --format`, `pull`, `rmi`, `cp`, `rename`, `stop`, `logs`, `info`, `ps -a`, `container inspect --format` | unchanged | Same behavior. |
| Windows `podman machine ssh -- sync` | none | Docker Desktop's VM is not reachable for a flush. Either the durability workaround is a no-op under Docker on Windows or a supported in-VM flush is found; see Open Questions. Linux keeps the host `sync`. |

Port conflicts are classified by the existing bind-failure markers, which already include Docker's `port is already allocated`. Startup diagnostics run the same commands and should name the engine in their header.

## UDFs under Docker

UDFs run through Nano's `nschroot` sandbox, which creates nested user, mount, and PID namespaces as an unprivileged process and chroots into the mounted SLC. Every failure in this path surfaces only as `VM crashed`, so each requirement below was found by elimination.

- **Seccomp must be unconfined.** Removing `seccomp=unconfined` reproduces `VM crashed` under both rootful and rootless Docker.
- **The data directory must be traversable by other users.** The launcher creates `local/runtime/exa` with mode `0750`. Under rootless engines the launcher user is the container's root, so this is invisible. Under rootful Docker the directory appears to the sandbox as owned by an unrelated UID, the unprivileged sandbox cannot enter `/exa/sandbox`, and UDFs crash. Making the directory `0755` at every start fixes new and existing deployments. Its parent stays `0750`, so other host users still cannot reach it.
- **Nano must run as container root.** Running Nano as the launcher user (`--user <uid>:<gid>`), which would avoid root-owned files, fails: the database starts and accepts a login, then the session fails with `FDTransport: Creation of named socket failed: Permission denied`.
- **SLCs must ship their mount points.** The sandbox creates the mount points it binds into the SLC root (`/proc`, `/dev`, `/sys`, `/buckets`, `/var/tmp`, and siblings) only when that root is writable. Podman's `rw=true` provides a per-container overlay for that; Docker has no equivalent. The official Python SLC ships all of them and runs UDFs. A custom SLC without them fails under Docker. Detecting the skeleton during custom SLC import and reporting it is a reasonable refinement.
- **Docker image mounts need a recent engine.** `--mount type=image` is not available in older Docker Engine releases. The minimum version should be established and checked during preparation.

## Ownership under Rootful Docker

Rootful Docker runs Nano as real root, so everything Nano creates below `local/runtime/exa` is owned by root: `bucketfs`, `jdbc`, `logs`, `storage`, `sandbox`, `certificates`, `slc`, and the configuration files. Everything is world-readable, and nothing outside `exa` becomes root-owned. Two user workflows break.

**Staging files.** User documentation directs users to write adapter and driver JARs into `bucketfs/bfsdefault/default` and drivers into `jdbc`, and the deployment directory is the only way to put files into BucketFS because its HTTP service is not published. Under root ownership these writes need sudo. The launcher therefore creates `bucketfs/bfsdefault/default` and `jdbc` as the launcher user, mode `0755`, before Nano first starts; Nano keeps them as found. Existing components are never changed, and a deployment whose directories Nano already created as root starts normally with a warning.

Requiring sudo was rejected. Docker group membership commonly exists without sudo on managed machines, files staged with sudo stay root-owned for every later edit, and the Podman path never needed it.

Pre-creating these directories does not widen access. Only the launcher user can write them, the `0750` parent keeps other users out, and symlinks placed in them resolve inside the container's mount namespace, where the only host paths are `exa` and the read-only SLC. A user in the `docker` group is already root-equivalent.

**Destroying.** `os.RemoveAll` cannot empty root-owned directories. When removal fails with a permission error, the launcher empties the data directory from inside a short-lived container and removes it again. Nano's image is distroless (only `/controller`), so it cannot do this; the cleanup uses `docker.io/library/busybox`, which should be pinned by digest (`sha256:73aaf090f3d85aa34ee199857f03fa3a95c8ede2ffd4cc2cdb5b94e566b11662` for 1.36.1). The image is pulled only on the first rootful destroy, so an offline host fails that destroy with the pull error.

Alternatives considered:

- A Docker named volume for `/exa` avoids root-owned host files and the helper image, but contradicts the requirement that Nano data is host-visible at `local/runtime/exa`.
- Handing ownership back with `chown` on every stop keeps the directory user-owned while stopped, but needs the same helper image on every stop and leaves files root-owned after a crash or reboot.
- Supporting only rootless Docker avoids all ownership work but excludes the most common Linux Docker installation.

## Windows

The Windows Docker preparer replaces the Podman availability, Windows Package Manager installation, and Podman machine steps with the readiness check in decision 4, so a host with Docker needs no approval. Bind-mounting a Windows path with the `:Z` suffix, file ownership as seen through Docker Desktop's filesystem sharing, and Docker Desktop's behavior when stopped are unverified.

Docker Desktop requires a paid subscription in larger organizations. The launcher only uses an installation that already exists, so it takes on no licensing decision for the user.

## Risks / Trade-offs

- **Weaker isolation under rootful Docker.** `seccomp=unconfined` and `apparmor=unconfined` on a container that runs as real root give a compromise of Nano or a UDF far more reach than under rootless Podman. A narrower custom seccomp profile that permits only the namespace syscalls the sandbox needs is the obvious hardening step.
- **Rootful Docker needs a root-equivalent group.** Membership in the `docker` group grants passwordless root. This is the user's choice of installation, but the documentation should say so.
- **Packaged Docker variants.** The Ubuntu snap package confines Docker's access to non-hidden paths in the home directory and gives it a private `/tmp`, which can break bind mounts and `load -i` for deployment or cache directories outside those paths. It was not verified and may need detection or documentation.
- **Custom SLCs lose UDF support** when they lack the directory skeleton.
- **Destroy may need network access** under rootful Docker, once, to pull the cleanup image.
- **Hosts with a Docker that cannot run Linux containers.** Windows CI runners are such hosts; their `docker` resolves but serves Windows containers only.
- **Engine selection can surprise.** A user who installs Docker after creating Podman deployments gets new deployments on Docker and old ones on Podman, and the same images are stored twice. `--container-engine podman` keeps new deployments on Podman.

## Testing

Launcher tests fake the container engine by placing an executable with the engine's name first on PATH. Such a fake is only consulted if the launcher selects that engine, and CI runners have Docker installed, so a Podman-only fake is bypassed silently: the launcher drives the real Docker instead, and a test either fails for an unrelated reason or passes without exercising the behavior it names. A test that asserts the absence of a container with the wrong engine's CLI passes vacuously in the same way. Tests that fake an engine should pin it with `--container-engine` and fake that engine, and live assertions should query the engine the deployment recorded.

## Migration Plan

No data conversion is required. Existing deployments have a runtime directory without an engine marker, are treated as Podman deployments, and behave exactly as before. Their `container_engine` reads as `auto` but cannot be changed to `docker` until they are destroyed. Only fresh deployments on hosts with Docker use Docker. Making the data directory `0755` applies to every direct-host deployment at its next start.

Rollback is downgrading the launcher. Podman deployments are unaffected. A Docker deployment cannot be operated by an older launcher, which would look for its container in Podman; the user destroys it with the newer launcher or removes the container with `docker rm -f exasol-db-<id>`.

## Open Questions

- Is `apparmor=unconfined` required on hosts that enforce AppArmor, and can a custom seccomp profile replace `seccomp=unconfined`?
- What is the minimum Docker Engine version that supports image mounts, and how should an older engine fail?
- How should Nano's startup writes be made durable on Windows under Docker Desktop, where the launcher cannot reach the VM's kernel?
- Does the Windows path work end to end with Docker Desktop, including the `:Z` mount suffix and filesystem sharing performance for the database?
