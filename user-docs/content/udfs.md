# UDFs and script language containers

User-defined functions (UDFs) execute in a script language container (SLC), which supplies a runtime
such as Python, Java, or R.

Cloud deployments include the standard SLCs. Local deployments in release 2.2 do not install one by
default. Install the language you need with `exasol slc`:

```bash
exasol slc list
exasol slc install python3
exasol slc install java
exasol slc install r
exasol slc update python3
exasol slc remove python3
```

The language argument is an alias used by `CREATE ... SCRIPT` and is matched case-insensitively. For
example, the Python container enables the `PYTHON3` and `PYTHON312` aliases, the Java container
enables `JAVA` and `JAVA17`, and the R container enables `R` and `R44`.

`exasol slc list` displays each available container's flavor, aliases, version, and installation
state.

Installing, updating, or removing an SLC normally restarts the local database. This drops open
connections and aborts running statements. The command asks for confirmation first:

- Use `--auto-approve` to skip the prompt. This is required for non-interactive use.
- Use `--no-restart` to record the change and activate it the next time the deployment starts.

The `install`, `update`, and `remove` SLC operations apply only to local deployments.
