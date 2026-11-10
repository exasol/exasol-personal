# Troubleshooting

Problems are grouped by the point at which they appear. Each entry names what you see, why it
happens, and how to resolve it.

## Collect diagnostics

Before changing anything, ask the launcher what it sees:

```bash
exasol status          # deployment state, safe at any time
exasol diag local      # JSON: runtime state, bound ports, reachability, database readiness
exasol diag info       # JSON: full deployment information
exasol diag cache      # JSON: runtime artifact cache state
```

`exasol status` reports one of the documented state values; see
[Interpret deployment status](scripting.md#interpret-deployment-status) for their meanings. The
`diag` commands only report, they never change a deployment.

Add `--json` to most commands for machine-readable output.

## Installing the launcher

### `exasol: command not found`

The installer places the `exasol` binary in `~/.local/bin`, which is not on every system's `PATH`.
Add the directory as instructed by the installer output, then open a new terminal so the change
takes effect. Verify with `exasol version`. See [Install the launcher](getting-started.md).

## Preparing a local host

### `docker is not available`

The deployment uses Docker, but the launcher cannot use it. The message includes Docker's own
error. Check that the daemon is running with `docker info`, that your user may use it (membership of
the `docker` group applies only to new login sessions), and that the active context in
`docker context ls` is the one you expect. If the message says the daemon runs Windows containers,
switch Docker Desktop to Linux containers. If it names a Docker Engine older than 29, upgrade
Docker. If it says the `docker` command reaches Podman, a Podman daemon answers in Docker's place;
point the `docker` command at Docker with `docker context use`, or select Podman. To use Podman
instead on a deployment that has not started yet, run
`exasol config set --container-engine podman`.

While Docker does not answer, `exasol status` reports `database_connection_failed` with a message
that the container engine is not answering, and `exasol start` and `exasol stop` fail with this
error. Once Docker answers again, it restarts a running deployment as described in
[Docker behaviour](local-deployment.md#docker-behaviour).

### `'podman' is required to run exasol locally on this platform`

On Linux the database runs directly in Podman on the host. Install Podman with your distribution's
package manager and make sure it is available in `PATH`. See
[System requirements](system-requirements.md#local-database).

### `rootless netns: kill network process: permission denied` on Ubuntu

Ubuntu 26.04 can deny Podman permission to stop its `pasta` network helper because of a missing
AppArmor rule ([Ubuntu bug 2154379](https://bugs.launchpad.net/ubuntu/+source/passt/+bug/2154379)).
`exasol stop` or `exasol destroy` then fails during container removal and can leave the deployment
network behind. Repeating the command before repairing the host policy does not resolve the cause.

Ask your administrator to check for the matching denial:

```bash
sudo journalctl -k | grep 'profile="pasta"' | grep 'operation="signal"'
```

The matching entry names `comm="podman"`, `signal=term`, `peer="podman"`, and
`apparmor="DENIED"`. Install a distribution update that fixes the linked bug when available.
Until then, an administrator can apply the
[runner-image maintainers' correction](https://github.com/actions/runner-images/issues/15182)
to `/etc/apparmor.d/usr.bin.pasta`: add this rule inside the `profile pasta` block, then reload it
with `sudo apparmor_parser -r /etc/apparmor.d/usr.bin.pasta`:

```text
signal (receive) set=(term) peer=podman,
```

This permits only receipt of SIGTERM from Podman's AppArmor profile. Keep AppArmor enabled.
The launcher does not change this host policy. After the administrator repairs it, run
`podman unshare --rootless-netns true` to verify network teardown, then repeat `exasol destroy`
for the affected deployment. Keep its deployment directory until destruction completes.

### A container engine is not installed and Windows Package Manager is unavailable

The launcher can install Podman or Docker Desktop on Windows, but only through `winget`. Install
App Installer from the Microsoft Store, or install Podman from [podman.io](https://podman.io/) or
[Docker Desktop](https://docs.docker.com/desktop/setup/install/windows-install/) manually, then run
the command again.

### `podman is not available on PATH` on Windows after installing it

Windows fixes a process's `PATH` when it starts, so a terminal opened before Podman was installed
never sees it. Open a new terminal and run the command again.

### `Docker Desktop did not run Linux containers within 10 minutes`

The launcher installed and started Docker Desktop, but its engine was not ready in time. Finish
starting Docker Desktop: accept its agreement if it shows one, and sign out and in again or restart
Windows if it asks. Once `docker info` answers, run the command again; the deployment uses Docker.

### `Podman answers on Docker's API pipe` after installing Docker Desktop

A Podman machine holds the pipe Docker Desktop needs, so Docker Desktop cannot start. Stop the
Podman machine with `podman machine stop`. If Podman was uninstalled, its machine can keep running;
end its `win-sshproxy` process, and remove the machine with
`wsl --unregister podman-machine-default` if you no longer need it. Then start Docker Desktop and
run the command again.

If the message instead says that `DOCKER_HOST` or a Docker context selects Podman, the `docker`
command connects to Podman rather than Docker Desktop. Unset `DOCKER_HOST`, or switch to Docker
Desktop with `docker context use desktop-linux`, and run the command again.

### `local runtime host preparation requires approval`

Installing a container engine changes shared host state, so the launcher asks before doing it. A
command with no terminal attached cannot ask and refuses instead of changing the host; when neither
engine is installed, the message is `no container engine is installed for local deployments`.
Re-run with `--auto-approve`, which installs the deployment's engine, or Podman when the deployment
is set to `auto`, or run the `winget` command the message shows yourself. To install the other
engine, select it first with `exasol config set --container-engine docker` or `podman`. See
[Windows host preparation](local-deployment.md#windows-host-preparation).

## Starting a local deployment

### Status is `stopped` after a Windows reboot

On Windows the deployment runs inside Podman's machine, which does not start again by itself when
the host reboots. `exasol status` then reports:

```text
Status: stopped
Message: Deployment stopped. Run `start` to restart or `destroy` to delete resources.
```

Start the deployment again, which also starts the Podman machine:

```bash
exasol start
```

### `local service "db" cannot bind configured host port`

Another process claimed the port the deployment saved during initialization. Stop the deployment,
assign a different port or let the launcher choose one, then start it again:

```bash
exasol stop
exasol config set --ports db:9563  # or: exasol config set --ports auto
exasol start
```

See [Run Exasol locally](local-deployment.md) for how the launcher selects and keeps the port.

### `Podman could not set up the deployment network (nftables error)`

Enabling a sidecar fails with this message, or `exasol start` and `exasol sidecar status` report it
for an enabled sidecar. The database keeps running, and local deployments without sidecars are not
affected.

The usual cause is a kernel without the nftables support that Podman needs for the
[deployment network](local-deployment.md#deployment-network), such as the one shipped with WSL 2.6,
so the database starts without it and sidecars cannot reach it. Sidecars on Podman need WSL 2.7 or
later. This applies to the Windows launcher and to the Linux
launcher inside WSL. Docker is not affected.

Rootful Podman cannot start the database without the network on these kernels, so
`exasol install local` and `exasol start` fail with the same message. The same fix applies.

Update WSL from a Windows terminal, restart the deployment, and enable the sidecar again:

```bash
wsl --update
exasol stop
exasol start
```

If you run the Linux launcher and cannot update WSL or the kernel, install `iptables`, switch
Podman to it in `~/.config/containers/containers.conf`, then run `exasol stop` and `exasol start`:

```toml
[network]
firewall_driver = "iptables"
```

The setting applies to all of your Podman containers.

### Connection details differ from the last session

Node addresses can change whenever a deployment restarts. Run `exasol info` to display the current
connection details.

## Installing to the cloud

### The provider rejects the credentials

The launcher uses the credentials your provider's CLI or environment already supplies, and reports
the provider's own authentication error. Confirm the credentials outside the launcher, then run the
installation again. See the setup guide for [AWS](cloud/aws.md), [Azure](cloud/azure.md),
[Exoscale](cloud/exoscale.md), [STACKIT](cloud/stackit.md), or [IONOS Cloud](cloud/ionos.md).

### Azure reports resource-provider registration errors

A new Azure subscription does not have every required resource provider registered. Register them
as described in [Configure subscription access](cloud/azure.md#configure-subscription-access), then
run the installation again.

### The installation was interrupted

Cloud resources created before an interruption are not removed automatically and can continue to
incur charges. Keep the deployment directory and retry the same installation command; the directory
records the selected presets and deployment state.

If you do not want to retry, remove the provisioned resources with `exasol destroy`. See
[Destroy a deployment](manage-deployments.md#destroy-a-deployment).

## Recovering a deployment

### Status is `operation_in_progress` but nothing is running

The deployment directory stays locked while a launcher operation runs, and a process that was
terminated can leave the lock behind. Confirm that no other launcher process is running, then
release the lock:

```bash
exasol diag unlock
```

Unlocking a directory that another process is still using can leave the deployment in an
inconsistent state, so check first.

### Resources were removed outside the launcher

If deployment resources no longer exist or cannot be reached through the launcher, remove only the
local deployment directory:

```bash
exasol remove
```

This does not destroy resources. If you deleted the deployment directory before destroying the
resources, remove the remaining resources through the cloud provider or local environment. See
[Destroy a deployment](manage-deployments.md#destroy-a-deployment).

## Features and runtimes

- **Runtime downloads:** for an invalid or interrupted runtime-tool download, follow the targeted
  cleanup steps under [Runtime cache](launcher-cache.md#clean-a-failed-download). The cache normally
  requires no maintenance.
- **UDFs and script language containers:** a container that cannot be activated is recorded as
  pending and the database still starts. See
  [UDFs and script language containers](udfs.md#install-a-custom-container).
- **Virtual schemas:** adapter, driver, and runtime errors are covered by
  [Virtual schemas](virtual-schemas.md#troubleshooting).
- **Missing capability:** some behavior differs between local and cloud deployments by design. Check
  [Limitations](limitations.md) before treating it as a fault.

## Get more help

Run `exasol <command> --help` for the options of an individual command. For further assistance, ask
in the [Exasol Community](https://community.exasol.com) using the `exasol-personal` tag, and include
the output of `exasol status` and `exasol diag local`.
