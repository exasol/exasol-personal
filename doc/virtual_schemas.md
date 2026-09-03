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
DEPLOY_DIR=$(exasol info --json | jq -r '.deploymentDir')
exasol slc install java --no-restart --deployment-dir "$DEPLOY_DIR"
```

The command above requires `jq`. It selects the deployment reported by `exasol info`; use the
same `--deployment` or `--deployment-dir` selector with `info` when working with a named or
explicitly selected deployment.

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

The launcher reports the active deployment directory in machine-readable form. Use this value so
files are staged for the deployment that the following commands operate on:

```bash
DEPLOY_DIR=$(exasol info --json | jq -r '.deploymentDir')
VS_DIR="$DEPLOY_DIR/local/runtime/vm-shared/vs"
mkdir -p "$VS_DIR"
```

For a named deployment or an explicitly selected directory, pass the same selector to `info`:

```bash
DEPLOY_DIR=$(exasol info --json --deployment demo | jq -r '.deploymentDir')
# or
DEPLOY_DIR=$(exasol info --json --deployment-dir /path/to/deployment | jq -r '.deploymentDir')
```

On Linux, Nano's `/exa` is the persistent Podman data directory on the host:

```bash
EXA_DATA="$DEPLOY_DIR/local/runtime/exa"
```

On macOS, the database runs inside a managed VM. The deployment's
`local/runtime/vm-shared` directory is mounted in the VM as `/mnt/host`. Use
`exasol shell host` to copy staged files into the database directories; do not rely on VM IP
addresses, SSH keys, or SSH ports.

## Worked example: PostgreSQL JDBC adapter

This example uses the [PostgreSQL virtual schema](https://github.com/exasol/postgresql-virtual-schema)
adapter. Check its releases page and the [PostgreSQL JDBC driver](https://jdbc.postgresql.org/download/)
site for current versions before downloading. The versions below are examples.

### 1. Select the deployment and download the adapter and driver

```bash
DEPLOY_DIR=$(exasol info --json | jq -r '.deploymentDir')
VS_DIR="$DEPLOY_DIR/local/runtime/vm-shared/vs"
mkdir -p "$VS_DIR"

ADAPTER_VERSION=4.0.0
ADAPTER_JAR=virtual-schema-dist-14.0.2-postgresql-4.0.0.jar
DRIVER_JAR=postgresql-42.7.13.jar

curl -L -o "$VS_DIR/$ADAPTER_JAR" \
  "https://github.com/exasol/postgresql-virtual-schema/releases/download/$ADAPTER_VERSION/$ADAPTER_JAR"
curl -L -o "$VS_DIR/$DRIVER_JAR" \
  "https://repo.maven.apache.org/maven2/org/postgresql/postgresql/42.7.13/$DRIVER_JAR"
```

Create the JDBC driver configuration in the same shared directory:

```bash
cat > "$VS_DIR/settings.cfg" <<EOF
DRIVERNAME=POSTGRESQL
PREFIX=jdbc:postgresql:
DRIVERMAIN=org.postgresql.Driver
FETCHSIZE=100000
INSERTSIZE=-1
JAR=$DRIVER_JAR

EOF
```

`settings.cfg` must end with an empty line. The driver JAR is needed in both BucketFS and the JDBC
directory because the adapter reads it for metadata and the ETL layer uses it for data transfer.

### 2. Copy the files into the database

On Linux:

```bash
mkdir -p "$EXA_DATA/bucketfs/bfsdefault/default/vs"
cp "$VS_DIR/$ADAPTER_JAR" "$VS_DIR/$DRIVER_JAR" \
  "$EXA_DATA/bucketfs/bfsdefault/default/vs/"
mkdir -p "$EXA_DATA/jdbc/POSTGRESQL"
cp "$VS_DIR/$DRIVER_JAR" "$VS_DIR/settings.cfg" "$EXA_DATA/jdbc/POSTGRESQL/"
```

On macOS:

```bash
exasol shell host --deployment-dir "$DEPLOY_DIR"
```

Inside the VM shell, run:

```bash
mkdir -p /var/lib/exa/bucketfs/bfsdefault/default/vs /var/lib/exa/jdbc/POSTGRESQL && \
cp /mnt/host/vs/virtual-schema-dist-14.0.2-postgresql-4.0.0.jar \
   /mnt/host/vs/postgresql-42.7.13.jar \
   /var/lib/exa/bucketfs/bfsdefault/default/vs/ && \
cp /mnt/host/vs/postgresql-42.7.13.jar \
   /mnt/host/vs/settings.cfg \
   /var/lib/exa/jdbc/POSTGRESQL/ && \
ls -lh /var/lib/exa/bucketfs/bfsdefault/default/vs /var/lib/exa/jdbc/POSTGRESQL
```

Then run `exit` to return to the macOS shell. `DRIVERNAME` is the name used in
`IMPORT ... DRIVER = '...'`. If either example filename changes, use the resulting filenames for
`JAR` and the `%jar` lines in the adapter script.

### 3. Apply the setup

```bash
exasol stop --deployment-dir "$DEPLOY_DIR"
exasol start --deployment-dir "$DEPLOY_DIR"
```

This activates the SLC and makes the JDBC driver configuration available to the database.

### Optional: run PostgreSQL inside the macOS VM

For a self-contained test, PostgreSQL can run inside the managed VM. Put it on a dedicated Podman
network and attach Nano to that network:

```bash
exasol shell host --deployment-dir "$DEPLOY_DIR"
```

Inside the VM shell, run:

```bash
EXASOL_CONTAINER=$(podman ps --format '{{.Names}}' | sed -n '/^exasol-db-/p' | head -n 1)
VS_NETWORK="vs-net-${EXASOL_CONTAINER#exasol-db-}"
podman network create "$VS_NETWORK"
podman network connect "$VS_NETWORK" "$EXASOL_CONTAINER"
podman run -d --rm \
  --name exasol-vs-postgres \
  --network "$VS_NETWORK" \
  --env POSTGRES_DB=vs_test \
  --env POSTGRES_USER=vs_test \
  --env POSTGRES_PASSWORD=vs_test \
  docker.io/library/postgres@sha256:c1b3783309b6499c795eed7c20135a1a4d25cae1b575c3d52c6f536129a1b109 \
  -c timezone=UTC
until podman exec exasol-vs-postgres pg_isready --username vs_test --dbname vs_test; do
  sleep 1
done
podman exec exasol-vs-postgres psql --no-password --username vs_test --dbname vs_test \
  -c "CREATE SCHEMA source_schema"
podman exec exasol-vs-postgres psql --no-password --username vs_test --dbname vs_test \
  -c "CREATE TABLE source_schema.source_data (id INTEGER PRIMARY KEY, payload TEXT)"
podman exec exasol-vs-postgres psql --no-password --username vs_test --dbname vs_test \
  -c "INSERT INTO source_schema.source_data VALUES (1, 'before-refresh')"
podman inspect exasol-vs-postgres \
  --format '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' \
  > /mnt/host/vs/postgres-ip
exit
```

The dedicated network is required because Nano and PostgreSQL use separate network namespaces. It
gives Nano a route to PostgreSQL while keeping PostgreSQL independent of Nano's network namespace.
Do not use `--network container:<Nano-container>`: that makes PostgreSQL dependent on Nano and can
prevent `exasol destroy` from removing the deployment until PostgreSQL is removed first.

The PostgreSQL address is written to the shared directory because Podman DNS aliases are not
available reliably in every managed VM environment. Use the address from `postgres-ip` in the
connection below. Remove the test container with `podman rm --force exasol-vs-postgres` inside
`exasol shell host` when it is no longer needed; it is also removed when the VM is destroyed.

### 4. Create the connection and virtual schema

Create the SQL file in the deployment directory. The connection host must be reachable from the
database VM or container; `localhost` refers to that environment, not necessarily to the machine
running the source database.

```bash
PG_HOST=$(tr -d '\r\n' < "$VS_DIR/postgres-ip")
cat > "$DEPLOY_DIR/create-vs.sql" <<EOF
CREATE OR REPLACE CONNECTION POSTGRES_CONN
  TO 'jdbc:postgresql://$PG_HOST:5432/vs_test'
  USER 'vs_test' IDENTIFIED BY 'vs_test';

CREATE SCHEMA IF NOT EXISTS VS;

CREATE OR REPLACE JAVA ADAPTER SCRIPT VS.POSTGRES_ADAPTER AS
  %scriptclass com.exasol.adapter.RequestDispatcher;
  %jvmoption -Duser.timezone=UTC;
  %jar /buckets/bfsdefault/default/vs/virtual-schema-dist-14.0.2-postgresql-4.0.0.jar;
  %jar /buckets/bfsdefault/default/vs/postgresql-42.7.13.jar;
/

CREATE VIRTUAL SCHEMA VS_POSTGRES
  USING VS.POSTGRES_ADAPTER
  WITH CONNECTION_NAME = 'POSTGRES_CONN'
       SCHEMA_NAME = 'source_schema';

SELECT * FROM VS_POSTGRES.source_data;
EOF
```

An external PostgreSQL instance is any source outside the managed Exasol VM. It may run on the
Mac host, another machine, or a cloud service. Replace the `PG_HOST` assignment with an address
reachable from the database VM, for example:

```bash
PG_HOST="192.168.1.20"
```

When PostgreSQL runs on the Mac host, do not use `127.0.0.1`: inside the Exasol VM that address
refers to the VM itself. Publish PostgreSQL on a reachable host interface and verify connectivity
from `exasol shell host` before creating the Virtual Schema.

For an external PostgreSQL instance, also change the database name, user, and password in the
connection definition to match that database. Change `SCHEMA_NAME` and the example table name to
match its schema and table.

Then run the SQL-file creation block again.

Run the SQL file with the same deployment selection:

```bash
exasol connect --deployment-dir "$DEPLOY_DIR" -f "$DEPLOY_DIR/create-vs.sql"
```

Use `ALTER VIRTUAL SCHEMA VS_POSTGRES REFRESH` when the source schema changes.

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
