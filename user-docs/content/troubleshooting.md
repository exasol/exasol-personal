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

### `'podman' is required to run exasol locally on this platform`

On Linux the database runs directly in Podman on the host. Install Podman with your distribution's
package manager and make sure it is available in `PATH`. See
[System requirements](system-requirements.md#local-database).

### Podman is not installed and Windows Package Manager is unavailable

The launcher can install Podman on Windows, but only through `winget`. Install App Installer from
the Microsoft Store, or install Podman manually from [podman.io](https://podman.io/), then run the
command again.

### `podman is not available on PATH` on Windows after installing it

Windows fixes a process's `PATH` when it starts, so a terminal opened before Podman was installed
never sees it. Open a new terminal and run the command again.

### `local runtime host preparation requires approval`

Installing Podman changes shared host state, so the launcher asks before doing it. A command with
no terminal attached cannot ask and refuses instead of changing the host. Re-run with
`--auto-approve`, or run the displayed setup command yourself. See
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

### Connection details differ from the last session

Node addresses can change whenever a deployment restarts. Run `exasol info` to display the current
connection details.

## Installing to the cloud

### The provider rejects the credentials

The launcher uses the credentials your provider's CLI or environment already supplies, and reports
the provider's own authentication error. Confirm the credentials outside the launcher, then run the
installation again. See the setup guide for [AWS](cloud/aws.md), [Azure](cloud/azure.md),
[Exoscale](cloud/exoscale.md), or [STACKIT](cloud/stackit.md).

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
