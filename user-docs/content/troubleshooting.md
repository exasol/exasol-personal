# Troubleshooting

## Inspect a local deployment

Run the local diagnostic command whether the deployment is running or stopped:

```bash
exasol diag local
```

It returns a JSON snapshot containing runtime state, bound host ports, reachability, and database
readiness.

Use `exasol info` to display the current connection details after a successful start. Node addresses
can change whenever a deployment restarts.

## Restart a local deployment on Windows after a reboot

On Windows the deployment runs inside Podman's machine, which does not start again by itself when
the host reboots. `exasol status` then reports:

```
Status: stopped
Message: Deployment stopped. Run `start` to restart or `destroy` to delete resources.
```

Start the deployment again, which also starts the Podman machine:

```bash
exasol start
```

## Recover from an interrupted installation

Cloud resources created before an interruption are not removed automatically and can continue to
incur charges. Keep the deployment directory and retry the same installation command. The directory
records the selected presets and deployment state.

If you do not want to retry, remove the provisioned resources with:

```bash
exasol destroy
```

After successful destruction, add `--remove` to remove the local deployment directory as well.

## Recover when resources were removed manually

If deployment resources no longer exist or cannot be reached through the launcher, remove only the
local deployment directory with:

```bash
exasol remove
```

This command does not destroy resources. If you deleted the deployment directory before destroying
the resources, remove the remaining resources through the cloud provider or local environment.

## Recover from a runtime download problem

If the launcher reports an invalid or interrupted runtime-tool download, follow the targeted cleanup
steps under [Runtime cache](launcher-cache.md#clean-a-failed-download). The cache normally requires
no user maintenance.

For help with an individual command, run `exasol <command> --help`. For further assistance, ask in
the [Exasol Community](https://community.exasol.com) using the `exasol-personal` tag.
