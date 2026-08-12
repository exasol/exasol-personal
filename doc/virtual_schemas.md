# Virtual schemas on local deployments

Virtual schemas let you query data that lives in another database as if it were a schema in
Exasol. This document covers the setup that a **local deployment** needs before the standard
Exasol virtual schema SQL flow works.

Cloud deployments need none of this — they ship the standard script language containers and
the usual driver locations, so follow the [Exasol virtual schema
documentation](https://docs.exasol.com/db/latest/database_concepts/virtual_schemas.htm)
directly.

The SQL flow itself is unchanged from that documentation. Local deployments only need
additional runtime and file setup, depending on the adapter you use.

## Runtime prerequisite

Local deployments ship without any script language container. Install the SLC required by
the virtual schema adapter before creating its adapter script. For example, JDBC-based
adapters are Java UDFs, and JDBC data transfer also needs a Java runtime:

```bash
exasol slc install java --no-restart
```

Use the corresponding language alias for adapters implemented in another supported language.
If the adapter needs packages that are not in an official SLC, install a custom SLC as
described in [UDFs and Script Language Containers](../README.md#-udfs-and-script-language-containers).

The `--no-restart` option records the SLC and applies it on the next start. The steps below
stage the adapter and driver files before that start, so the complete setup requires one
restart. If the required SLC is already installed, skip this command.

## Quick setup

For a JDBC-based virtual schema, the complete local-deployment flow is:

1. Install the required adapter SLC with `--no-restart`.
2. Download the adapter and source-database JDBC driver.
3. Copy both JARs into BucketFS.
4. Register the JDBC driver under `/exa/jdbc/<DRIVERNAME>/`.
5. Restart once with `exasol stop && exasol start`.
6. Run the standard Exasol SQL to create the connection, adapter script, and virtual schema.

The PostgreSQL example below gives the exact commands for each step.

## JDBC adapter setup

The rest of this guide uses a JDBC adapter as an example. A JDBC virtual schema needs three
files: the virtual schema adapter JAR, the JDBC driver JAR for your source database, and a
`settings.cfg` describing the driver. They live in two different places, because two
different components read them.

| File | Location inside the database container | Read by |
| --- | --- | --- |
| Adapter JAR + driver JAR | `/exa/bucketfs/<service>/<bucket>/<dir>/` | the adapter script, through `%jar /buckets/...` |
| Driver JAR + `settings.cfg` | `/exa/jdbc/<DRIVERNAME>/` | the ETL layer, for `IMPORT`/`EXPORT` |

The driver JAR is needed in both places: the adapter uses it on its own classpath to read
metadata, and the ETL layer uses the registered copy to transfer data.

> **Difference from the Exasol documentation.** Exasol's [Add JDBC
> Driver](https://docs.exasol.com/db/latest/administration/on-premise/manage_drivers/add_jdbc_driver.htm)
> page places the driver and `settings.cfg` in BucketFS under `drivers/jdbc`. Local
> deployments use `/exa/jdbc/<DRIVERNAME>/` instead, which is where the local database looks
> for JDBC driver configuration. Everything else on that page — the `settings.cfg` keys and
> their meaning — applies unchanged.

Any directory you create under `/exa/bucketfs` becomes a bucket, so you can use the same
bucket and path names as the Exasol examples and keep your SQL identical to the
documentation.

## Connection details for local deployments

The database runs inside a VM. `exasol shell host` opens a shell in that VM, and its SSH key
and port are in the deployment directory, so you can copy files in with `scp`:

```bash
# Deployment directory paths
KEY=./local/node_access.pem
PORT=$(jq -r '.ports.ssh' ./local/runtime/vm-state.json)
```

Create the target directories, then copy the files in. The worked example below stages both
the SLC and the JDBC files before restarting the deployment once.

## Worked example: PostgreSQL JDBC adapter

This example uses the [PostgreSQL virtual
schema](https://github.com/exasol/postgresql-virtual-schema) adapter. Check its releases page
and the [PostgreSQL JDBC driver](https://jdbc.postgresql.org/download/) site for current
versions before downloading. The versions below are examples and may need to be updated.

### 1. Download the adapter and driver

```bash
# Example versions; replace these with current releases when needed.
ADAPTER_VERSION=4.0.0
ADAPTER_JAR=virtual-schema-dist-14.0.2-postgresql-4.0.0.jar
DRIVER_JAR=postgresql-42.7.13.jar

curl -L -o "$ADAPTER_JAR" \
  "https://github.com/exasol/postgresql-virtual-schema/releases/download/$ADAPTER_VERSION/$ADAPTER_JAR"
curl -L -o "$DRIVER_JAR" "https://jdbc.postgresql.org/download/$DRIVER_JAR"
```

### 2. Copy both JARs into BucketFS

```bash
ssh -i "$KEY" -p "$PORT" root@127.0.0.1 \
  'mkdir -p /var/lib/exa/bucketfs/bfsdefault/default/vs'

scp -i "$KEY" -P "$PORT" \
  "$ADAPTER_JAR" "$DRIVER_JAR" \
  root@127.0.0.1:/var/lib/exa/bucketfs/bfsdefault/default/vs/
```

Inside the database, `/var/lib/exa` is `/exa`, and BucketFS contents are reachable under
`/buckets`. So these files are addressed in SQL as
`/buckets/bfsdefault/default/vs/<filename>`.

### 3. Register the JDBC driver

```bash
ssh -i "$KEY" -p "$PORT" root@127.0.0.1 'mkdir -p /var/lib/exa/jdbc/POSTGRESQL'

scp -i "$KEY" -P "$PORT" "$DRIVER_JAR" \
  root@127.0.0.1:/var/lib/exa/jdbc/POSTGRESQL/

ssh -i "$KEY" -p "$PORT" root@127.0.0.1 'cat > /var/lib/exa/jdbc/POSTGRESQL/settings.cfg <<CFG
DRIVERNAME=POSTGRESQL
PREFIX=jdbc:postgresql:
DRIVERMAIN=org.postgresql.Driver
FETCHSIZE=100000
INSERTSIZE=-1
JAR=postgresql-42.7.13.jar

CFG'
```

`DRIVERNAME` is the name you use in `IMPORT ... DRIVER = '...'`. The file must end with an
empty line. If you change either example variable, use the resulting filenames for `JAR`
and the `%jar` lines in the adapter script below.

### 4. Apply the setup

```bash
exasol stop && exasol start
```

This activates the SLC and makes the JDBC driver configuration available to the database.

### 5. Create the connection and the virtual schema

From here everything is standard Exasol SQL:

```sql
CREATE OR REPLACE CONNECTION POSTGRES_CONN
  TO 'jdbc:postgresql://<host>:5432/<database>'
  USER '<user>' IDENTIFIED BY '<password>';

CREATE SCHEMA IF NOT EXISTS VS;

CREATE OR REPLACE JAVA ADAPTER SCRIPT VS.POSTGRES_ADAPTER AS
  %scriptclass com.exasol.adapter.RequestDispatcher;
  %jar /buckets/bfsdefault/default/vs/virtual-schema-dist-14.0.2-postgresql-4.0.0.jar;
  %jar /buckets/bfsdefault/default/vs/postgresql-42.7.13.jar;
/

CREATE VIRTUAL SCHEMA VS_POSTGRES
  USING VS.POSTGRES_ADAPTER
  WITH CONNECTION_NAME = 'POSTGRES_CONN'
       SCHEMA_NAME = 'public';

SELECT * FROM VS_POSTGRES.<table>;
```

Use `ALTER VIRTUAL SCHEMA VS_POSTGRES REFRESH` when the source schema changes.

If the source database runs on your own machine rather than in the VM, use the VM's gateway
address as `<host>` rather than `localhost`.

## Other adapters and JDBC dialects

For non-JDBC adapters, install the SLC required by the adapter and follow that adapter's
installation documentation for its dependencies and script definition. The [Exasol virtual
schema
documentation](https://docs.exasol.com/db/latest/database_concepts/virtual_schemas.htm) lists
the available adapters and their properties.

The same file pattern applies to every JDBC-based dialect — Oracle, SQL Server, Snowflake,
MySQL, BigQuery and the rest. Swap the adapter JAR, the driver JAR, and the `settings.cfg`
values (`DRIVERNAME`, `PREFIX`, `DRIVERMAIN`, `JAR`) for your source database.

## Troubleshooting

**`ETL-1014: No default DRIVER registered`** — the driver is not registered. Check that
`/exa/jdbc/<DRIVERNAME>/` contains both the JAR and `settings.cfg`, that `settings.cfg` ends
with an empty line, and that you restarted the deployment.

**Adapter script fails to find its class** — check the `%jar` paths resolve under `/buckets`
and that the files were copied into the bucket directory.

**No runtime errors during adapter execution or `IMPORT`** — confirm that the SLC required
by the adapter is installed with `exasol slc list`. For JDBC adapters, this is the Java SLC.
