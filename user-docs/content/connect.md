# Connect to the database

Run `exasol info` after installation to display database connection details. The deployment's
`secrets.json` contains the credentials needed by database clients.

See [Connect to Exasol](https://docs.exasol.com/db/latest/connect_exasol.htm) for supported drivers and
client applications.

## Use the built-in SQL client

Start an interactive SQL session:

```bash
exasol connect
```

Run one or more semicolon-separated statements without opening an interactive session:

```bash
exasol connect -c "SELECT 1; SELECT 2"
```

Run statements from a file:

```bash
exasol connect -f script.sql
```

`--command` and `--file` are mutually exclusive. Non-interactive execution stops at the first failed
statement and exits with a non-zero status.

## Select the output format

Use `--csv` for CSV output:

```bash
exasol connect --csv -c "SELECT * FROM PRODUCTS" > products.csv
```

Use `--json` for machine-readable output. A non-interactive invocation writes one JSON document for
all statements, including SQL errors. An interactive `exasol connect --json` session writes one JSON
document per statement.

## Limit query output

Interactive query output is limited to 100 rows by default. Piped input and `--command` or `--file`
return the full result unless you set a limit.

```bash
exasol connect --max-rows 0
echo "SELECT * FROM PRODUCTS;" | exasol connect --max-rows 1000
```

Use `--max-rows 0` for unlimited interactive output.

For SQL syntax, functions, and data types, see the
[Exasol SQL reference](https://docs.exasol.com/db/latest/sql_reference.htm).

## Use Exasol Admin

Exasol Admin is available for cloud deployments. The installation output and `exasol info` show its
URL. Its credentials are stored in `secrets.json`.

The browser can display a warning because Exasol Admin uses a self-signed certificate. Verify that
you are connecting to your deployment before accepting the certificate warning.

Exasol Admin is not available for local deployments.

## Open a shell

Open a shell on a cloud compute instance or in the managed macOS virtual machine:

```bash
exasol shell host
```

Open a shell inside the database container:

```bash
exasol shell container
```

The runtime-managed shell commands are not available for local deployments on Linux or Windows.
