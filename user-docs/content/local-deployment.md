# Run Exasol locally

Release 2.2 supports local deployments on Apple Silicon Macs running macOS 15 or later with at
least 8 GB of memory.

After [installing the launcher](getting-started.md), run:

```bash
exasol install local
```

The launcher creates a managed local virtual machine and starts a single-node Exasol database. Run
`exasol info` at any time to display connection information.

The virtual machine uses about half of the detected host memory by default. An explicitly configured
memory value must be at least 4096 MB. The initial local database credentials are `sys` / `exasol`.

## Open a shell

Open a shell in the managed virtual machine:

```bash
exasol shell host
```

Open a shell inside the database container:

```bash
exasol shell container
```

## Next steps

- [Connect to the database](connect.md).
- [Load sample data](load-data.md).
- [Install a script language container](udfs.md) to run UDFs.
- [Manage the deployment lifecycle](manage-deployments.md).

If the local deployment does not behave as expected, see [Troubleshooting](troubleshooting.md).
