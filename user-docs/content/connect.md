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

For scripts, including the structured error and process exit-status contract, see
[Scripting the launcher](scripting.md#execute-sql-non-interactively).

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

## Access the deployment host

Open an interactive host shell, or pass a command after `--` to run it directly:

```bash
exasol shell host
exasol shell host -- ip route
```

Host shell access is available for cloud deployments and macOS local deployments.

## Use Exasol Admin

Exasol Admin is available for cloud deployments. The installation output and `exasol info` show its
URL. Its credentials are stored in `secrets.json`.

The browser can display a warning because Exasol Admin uses a self-signed certificate. Verify that
you are connecting to your deployment before accepting the certificate warning.

Exasol Admin is not available for local deployments.

## Access BucketFS on cloud deployments

New deployments initialized with the current built-in cloud presets store credentials for service
`bfsdefault`, bucket `default`, in `secrets.json`. The optional `bucketfs` object is keyed by service
name; each service's `buckets` object is keyed by bucket name. Each bucket contains `read` and `write`
objects with `username` and `password` fields. The default usernames are `r` and `w`; the write
credential permits both reading and writing.

Use `exasol info` to find the deployment directory and the node address. From a source permitted by
`allowed_cidr`, upload a file to `https://<node-address>:2581/default/<path>`:

```bash
DEPLOY_DIR=$(exasol info --json | jq -r '.deploymentDir')
NODE=$(jq -r '.nodes.n11.publicIp' "$DEPLOY_DIR/deployment.json")
BUCKET_USER=$(jq -r '.bucketfs.bfsdefault.buckets.default.write.username' "$DEPLOY_DIR/secrets.json")
curl --fail --insecure --user "$BUCKET_USER" --upload-file ./example.jar \
  "https://$NODE:2581/default/example.jar"
```

This example requires `jq`. Curl prompts for the password: use
`bucketfs.bfsdefault.buckets.default.write.password` from `secrets.json`. The example accepts the
deployment's self-signed certificate with `--insecure`; verify that the address belongs to your
deployment. Pass the same deployment selector to `info` when using a named deployment.

The launcher establishes these credentials during installation and retains them across stop/start.
Older deployment directories retain their extracted presets; a launcher upgrade does not add these
credentials. Changes made outside the launcher are not synchronized back to `secrets.json`.

For additional buckets and access policy, see the
[BucketFS overview](https://docs.exasol.com/db/latest/administration/on-premise/bucketfs/bucketfs_overview.htm).
For SQL connections and grants, see
[Database access to BucketFS](https://docs.exasol.com/db/latest/administration/on-premise/bucketfs/database_access.htm).
Local deployments use [filesystem-backed storage](virtual-schemas.md#where-local-deployments-keep-database-files)
without BucketFS HTTP credentials.
