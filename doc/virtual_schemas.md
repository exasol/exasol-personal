# Virtual schemas on local deployments

Virtual schemas let you query data in another database as if it were a schema in Exasol. This
guide covers the setup a local deployment needs before the standard Exasol virtual schema SQL
flow works.

Cloud deployments already include the standard script language containers and driver locations,
so follow the [Exasol virtual schema documentation](https://docs.exasol.com/db/latest/database_concepts/virtual_schemas.htm)
directly for those deployments.

## Runtime prerequisite

Local deployments ship without a script language container. Install the SLC required by the
virtual schema adapter before creating its adapter script. JDBC-based adapters are Java UDFs, and
JDBC data transfer also needs a Java runtime:

```bash
exasol slc install java --no-restart
```

Use the corresponding language alias for adapters implemented in another supported language. If
the adapter needs packages that are not in an official SLC, install a custom SLC as described in
[UDFs and Script Language Containers](../README.md#-udfs-and-script-language-containers).

The `--no-restart` option records the SLC and applies it on the next start. The steps below stage
the adapter and driver files before that start, so the complete setup requires one restart. Skip
this command if the required SLC is already installed.

## Quick setup

For a JDBC-based virtual schema:

1. Install the Java SLC with `--no-restart`.
2. Download the adapter and source-database JDBC driver.
3. Copy both JARs into BucketFS.
4. Register the JDBC driver under `/exa/jdbc/<DRIVERNAME>/`.
5. Restart once with `exasol stop && exasol start`.
6. Run the standard Exasol SQL to create the connection, adapter script, and virtual schema.

The PostgreSQL example below gives the commands for each step.

## JDBC adapter setup

A JDBC virtual schema needs the virtual schema adapter JAR, the JDBC driver JAR for the source
database, and a `settings.cfg` describing the driver. They live in two locations because separate
components read them.

| File | Location inside the database container | Read by |
| --- | --- | --- |
| Adapter JAR and driver JAR | `/exa/bucketfs/<service>/<bucket>/<dir>/` | the adapter script, through `%jar /buckets/...` |
| Driver JAR and `settings.cfg` | `/exa/jdbc/<DRIVERNAME>/` | the ETL layer, for `IMPORT` and `EXPORT` |

The driver JAR is needed in both places: the adapter uses it on its classpath to read metadata,
and the ETL layer uses the registered copy to transfer data.

Exasol's [Add JDBC Driver](https://docs.exasol.com/db/latest/administration/on-premise/manage_drivers/add_jdbc_driver.htm)
guide places the driver and `settings.cfg` in BucketFS under `drivers/jdbc`. Local deployments use
`/exa/jdbc/<DRIVERNAME>/` instead. The `settings.cfg` keys and their meaning remain the same.

BucketFS contents are reachable from adapter scripts under `/buckets`, so files stored in
`/exa/bucketfs/bfsdefault/default/vs/` are addressed in SQL as
`/buckets/bfsdefault/default/vs/<filename>`.

## Accessing local deployment files

Run the commands below from the deployment directory. On Linux, Nano's `/exa` is the persistent
Podman data directory on the host:

```bash
EXA_DATA=./local/runtime/exa
```

On macOS, the database runs inside the managed VM. Use its SSH key and forwarded port to copy
files into the VM:

```bash
KEY=./local/node_access.pem
PORT=$(jq -r '.ports.ssh' ./local/runtime/vm-state.json)
```

## Worked example: PostgreSQL JDBC adapter

This example uses the [PostgreSQL virtual schema](https://github.com/exasol/postgresql-virtual-schema)
adapter. Check its releases page and the [PostgreSQL JDBC driver](https://jdbc.postgresql.org/download/)
site for current versions before downloading. The versions below are examples.

### 1. Download the adapter and driver

```bash
ADAPTER_VERSION=4.0.0
ADAPTER_JAR=virtual-schema-dist-14.0.2-postgresql-4.0.0.jar
DRIVER_JAR=postgresql-42.7.13.jar

curl -L -o "$ADAPTER_JAR" \
  "https://github.com/exasol/postgresql-virtual-schema/releases/download/$ADAPTER_VERSION/$ADAPTER_JAR"
curl -L -o "$DRIVER_JAR" "https://jdbc.postgresql.org/download/$DRIVER_JAR"
```

### 2. Copy both JARs into BucketFS

On Linux:

```bash
mkdir -p "$EXA_DATA/bucketfs/bfsdefault/default/vs"
cp "$ADAPTER_JAR" "$DRIVER_JAR" "$EXA_DATA/bucketfs/bfsdefault/default/vs/"
```

On macOS:

```bash
ssh -i "$KEY" -p "$PORT" root@127.0.0.1 \
  'mkdir -p /var/lib/exa/bucketfs/bfsdefault/default/vs'

scp -i "$KEY" -P "$PORT" \
  "$ADAPTER_JAR" "$DRIVER_JAR" \
  root@127.0.0.1:/var/lib/exa/bucketfs/bfsdefault/default/vs/
```

### 3. Register the JDBC driver

On Linux:

```bash
mkdir -p "$EXA_DATA/jdbc/POSTGRESQL"
cp "$DRIVER_JAR" "$EXA_DATA/jdbc/POSTGRESQL/"
cat > "$EXA_DATA/jdbc/POSTGRESQL/settings.cfg" <<CFG
DRIVERNAME=POSTGRESQL
PREFIX=jdbc:postgresql:
DRIVERMAIN=org.postgresql.Driver
FETCHSIZE=100000
INSERTSIZE=-1
JAR=postgresql-42.7.13.jar

CFG
```

On macOS:

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

`DRIVERNAME` is the name used in `IMPORT ... DRIVER = '...'`. The file must end with an empty
line. If either example filename changes, use the resulting filenames for `JAR` and the `%jar`
lines in the adapter script.

### 4. Apply the setup

```bash
exasol stop && exasol start
```

This activates the SLC and makes the JDBC driver configuration available to the database.

### 5. Create the connection and virtual schema

From here, the SQL is the standard Exasol flow:

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

Use `ALTER VIRTUAL SCHEMA VS_POSTGRES REFRESH` when the source schema changes. The connection host
must be reachable from the database container or VM; `localhost` refers to that environment, not
necessarily to the machine running the source database.

## Other adapters and JDBC dialects

For non-JDBC adapters, install the SLC required by the adapter and follow its installation
documentation for dependencies and script definition. The [Exasol virtual schema documentation](https://docs.exasol.com/db/latest/database_concepts/virtual_schemas.htm)
lists the available adapters and their properties.

The same file pattern applies to other JDBC-based dialects. Substitute the adapter JAR, driver
JAR, and `settings.cfg` values (`DRIVERNAME`, `PREFIX`, `DRIVERMAIN`, and `JAR`) for the source
database.

## Troubleshooting

**`ETL-1014: No default DRIVER registered`** — check that `/exa/jdbc/<DRIVERNAME>/` contains the
JAR and `settings.cfg`, that `settings.cfg` ends with an empty line, and that the deployment was
restarted.

**Adapter script cannot find its class** — check that the `%jar` paths resolve under `/buckets`
and that the files were copied into the bucket directory.

**Adapter execution or `IMPORT` reports no runtime** — confirm that the adapter's SLC is installed
with `exasol slc list`. JDBC adapters require the Java SLC.
