# Run Exasol locally

Local deployments run on supported macOS, Linux, and Windows computers.

- On macOS, the launcher manages a virtual machine containing Podman and the database. An Apple
  Silicon Mac with at least 8 GB of memory is required.
- On Linux, the database runs directly on the host in Docker or Podman. See
  [Container engine](#container-engine).
- On Windows, the database runs in Docker Desktop with Linux containers, or in Podman's default
  machine, which requires Windows Package Manager (`winget`) and the prerequisites for a Podman
  machine.

After [installing the launcher](getting-started.md), run:

```bash
exasol install local
```

The launcher starts a single-node Exasol database. Run `exasol info` at any time to display
connection information.

The launcher selects and saves a database port during initialization, preferring `8563`, and keeps
it stable across restarts. To choose one explicitly, run `exasol install local --ports db:9563`.
To change the port later, stop the deployment first, update the configuration, and restart it:

```bash
exasol stop
exasol config set --ports db:9563  # or: exasol config set --ports auto
exasol start
```

If another process claims the saved port while the deployment is stopped, startup reports the
conflict and shows the same explicit and automatic replacement commands.

On macOS, the managed virtual machine uses about half of the detected host memory by default. An
explicitly configured memory value must be at least 4096 MB. Linux and Windows use host-managed
resources, so the virtual-machine sizing options do not apply.

Each new local deployment initializes the `sys` database user with a unique, randomly generated
password and stores it in the deployment's `secrets.json`. `exasol connect` uses it automatically.
Other SQL clients can connect as `sys` with the password from that file. Local deployments created
by earlier launcher versions keep the credential they were created with.

BucketFS HTTP access is not available for local deployments. BucketFS storage remains available
through the deployment directory, as described in [Virtual schemas](virtual-schemas.md).

## Deployment network

Each new database container joins a named bridge network owned by its
deployment, where it is reachable as `database`. The published database port
remains the port saved in the deployment configuration. Existing containers
adopt the deployment network after an explicit `exasol stop` followed by
`exasol start`. Destroying the deployment removes its network.

## Container engine

On Linux and Windows, a local deployment runs on Docker or Podman. The `container_engine` option
chooses which:

- `auto` (the default) uses Podman when it is installed, and otherwise Docker when it is installed
  and runs Linux containers. With neither, it uses Podman, which the launcher offers to install on
  Windows. A Podman command installed under the name `docker` counts as Podman.
- `docker` or `podman` uses that engine. The launcher never installs or starts Docker; if Docker
  cannot be used, the command fails and explains what to check.

```bash
exasol install local --container-engine podman
exasol config set --container-engine docker   # before the first start
```

A deployment keeps the engine it first started on, even if Docker is installed or removed later.
Deployments created by earlier launcher versions keep using Podman. To change the engine of a
started deployment, destroy it first, which deletes its data. `exasol diag local` reports the
engine a deployment uses; `exasol config get` reports the configured choice.

A Docker deployment uses your current Docker context. Keep using the same context for the
deployment, because a different one points the launcher at a daemon without its containers.

### Docker behaviour

- **Reboots:** Docker restarts a running deployment's database and sidecars when its daemon starts
  again, for example after a reboot. This applies once the deployment has been stopped and started
  again with `exasol stop` and `exasol start` after `install` or `deploy`. A sidecar whose
  `restartPolicy` is `OnFailure` or `Never` stays stopped until the next `exasol start`. A
  deployment you stopped stays stopped. While the daemon does not answer, `exasol status` reports
  that the container engine is not answering, and `exasol start` and `exasol stop` fail until it
  answers. With Podman, a deployment that was running is reported as stopped after a reboot until
  you run `exasol start`.
- **File ownership:** when Docker runs containers as root, the database's files below
  `local/runtime/exa` belong to root. They stay readable, the BucketFS and JDBC directories you
  copy files into stay writable, and `exasol destroy` removes everything without `sudo`. The first
  such destroy downloads a small helper image.
- **Permissions:** membership of the `docker` group grants root-equivalent access to the host.
  Rootless Docker avoids this.
- **Isolation:** UDFs need Docker to run the database container without its default seccomp and
  AppArmor profiles and with the system paths below `/proc` and `/sys` unmasked. On hosts where
  AppArmor restricts unprivileged user namespaces, such as Ubuntu 24.04, rootful Docker also gives
  the database container the `SYS_ADMIN` capability. A compromised database or UDF therefore has
  more access to the host than under Podman, and under rootful Docker that access is root's.
  Rootless Docker limits it to your user.
- **Custom SLCs:** Docker cannot give a script language container a writable overlay, so a
  [custom SLC](udfs.md#install-a-custom-container) must include the directories the UDF sandbox
  mounts into, as official SLCs do.

## Windows host preparation

This section applies to deployments that use Podman. If Podman is missing, the launcher can offer
to install it. It also creates or starts Podman's
default machine when necessary, without changing the configuration of an existing machine. Because
installing Podman changes shared host state, the launcher displays the exact command and asks for
approval.

Use `--auto-approve` for unattended preparation. A command that cannot prompt refuses this host
change unless the option is present, unlike other confirmations, which an unattended run approves
automatically. Stopping or destroying an Exasol deployment leaves the shared Podman machine
running.

## Next steps

- [Connect to the database](connect.md).
- [Load sample data](load-data.md).
- [Install a script language container](udfs.md) to run UDFs.
- [Manage the deployment lifecycle](manage-deployments.md).

If the local deployment does not behave as expected, see [Troubleshooting](troubleshooting.md).
