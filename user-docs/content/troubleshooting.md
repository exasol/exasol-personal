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

## Inspect the runtime cache

The launcher downloads runtime tools on demand. Inspect their cached state without changing it:

```bash
exasol diag cache
```

Remove artifacts with invalid checksums or partial downloads:

```bash
exasol cache clean --invalid --dry-run
exasol cache clean --invalid
exasol cache clean --partial-downloads --dry-run
exasol cache clean --partial-downloads
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

## Check for newer versions

Exasol Personal periodically checks for a newer launcher version. Cloud deployments also check for a
newer database version. Disable these checks during installation when required:

```bash
exasol install <preset> --no-launcher-version-check
exasol install <preset> --no-db-version-check
```

The database-version option applies to cloud deployments.

For help with an individual command, run `exasol <command> --help`. For further assistance, ask in
the [Exasol Community](https://community.exasol.com) using the `exasol-personal` tag.
