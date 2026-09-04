# Virtual schemas on local deployments

Virtual schemas let you query another database as if its objects were an Exasol schema. Cloud
deployments already include the standard script language containers and driver locations, so follow
the [Exasol virtual schema documentation](https://docs.exasol.com/db/latest/database_concepts/virtual_schemas.htm)
for those deployments.

Local deployments require you to install the adapter runtime and stage its dependencies. This guide
uses the PostgreSQL JDBC adapter as an example.

## Install the adapter runtime

JDBC adapters execute as Java UDFs and JDBC data transfer needs a Java runtime. Install the Java SLC
without restarting yet:

```bash
DEPLOY_DIR=$(exasol info --json | jq -r '.deploymentDir')
exasol slc install java --no-restart --deployment-dir "$DEPLOY_DIR"
```

This command requires `jq`. Use the same `--deployment` or `--deployment-dir` selector with `info`
when working with a named or explicitly selected deployment. Skip the installation if the required
SLC is already present. For an adapter implemented in another language, install that runtime. A
[custom SLC](udfs.md#install-a-custom-container) can supply packages missing from the official one.

## Understand the required files

A JDBC virtual schema needs the adapter JAR, the source database's JDBC driver JAR, and a
`settings.cfg` describing the driver.

| Files | Container location | Used by |
| --- | --- | --- |
| Adapter and driver JARs | `/exa/bucketfs/<service>/<bucket>/<dir>/` | Adapter script through `/buckets/...` |
| Driver JAR and `settings.cfg` | `/exa/jdbc/<DRIVERNAME>/` | ETL layer for `IMPORT` and `EXPORT` |

The driver JAR is present in both locations because the adapter reads it for metadata and the ETL
layer uses the registered copy for data transfer. A BucketFS file stored at
`/exa/bucketfs/bfsdefault/default/vs/example.jar` is addressed by an adapter script as
`/buckets/bfsdefault/default/vs/example.jar`.

## Select the deployment and staging directory

Resolve the deployment directory and prepare a staging location:

```bash
DEPLOY_DIR=$(exasol info --json | jq -r '.deploymentDir')
VS_DIR="$DEPLOY_DIR/local/runtime/vm-shared/vs"
mkdir -p "$VS_DIR"
```

For another deployment selection:

```bash
DEPLOY_DIR=$(exasol info --json --deployment demo | jq -r '.deploymentDir')
# or
DEPLOY_DIR=$(exasol info --json --deployment-dir /path/to/deployment | jq -r '.deploymentDir')
```

On Linux, the persistent database data is directly accessible on the host:

```bash
EXA_DATA="$DEPLOY_DIR/local/runtime/exa"
```

On macOS, `local/runtime/vm-shared` is mounted inside the managed virtual machine as `/mnt/host`.
Use `exasol shell host` to copy staged files into database directories. Do not depend on the virtual
machine's IP address, SSH key, or SSH port.

## PostgreSQL JDBC example

Check the [PostgreSQL virtual schema releases](https://github.com/exasol/postgresql-virtual-schema)
and [PostgreSQL JDBC downloads](https://jdbc.postgresql.org/download/) for current versions. The
versions below are examples and should be updated together when newer compatible artifacts are used.

### Download the adapter and driver

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

Create `$VS_DIR/settings.cfg` with an empty final line:

```text
DRIVERNAME=POSTGRESQL
PREFIX=jdbc:postgresql:
DRIVERMAIN=org.postgresql.Driver
FETCHSIZE=100000
INSERTSIZE=-1
JAR=postgresql-42.7.13.jar

```

### Copy the files on Linux

```bash
mkdir -p "$EXA_DATA/bucketfs/bfsdefault/default/vs"
cp "$VS_DIR/$ADAPTER_JAR" "$VS_DIR/$DRIVER_JAR" \
  "$EXA_DATA/bucketfs/bfsdefault/default/vs/"
mkdir -p "$EXA_DATA/jdbc/POSTGRESQL"
cp "$VS_DIR/$DRIVER_JAR" "$VS_DIR/settings.cfg" "$EXA_DATA/jdbc/POSTGRESQL/"
```

### Copy the files on macOS

Open the virtual-machine shell:

```bash
exasol shell host --deployment-dir "$DEPLOY_DIR"
```

Then run inside it:

```bash
mkdir -p /var/lib/exa/bucketfs/bfsdefault/default/vs /var/lib/exa/jdbc/POSTGRESQL
cp /mnt/host/vs/virtual-schema-dist-14.0.2-postgresql-4.0.0.jar \
   /mnt/host/vs/postgresql-42.7.13.jar \
   /var/lib/exa/bucketfs/bfsdefault/default/vs/
cp /mnt/host/vs/postgresql-42.7.13.jar \
   /mnt/host/vs/settings.cfg \
   /var/lib/exa/jdbc/POSTGRESQL/
ls -lh /var/lib/exa/bucketfs/bfsdefault/default/vs /var/lib/exa/jdbc/POSTGRESQL
```

Exit the shell after verifying the files. If you selected other artifact versions, substitute their
filenames throughout the setup.

### Restart once

Apply the pending SLC and JDBC driver configuration:

```bash
exasol stop --deployment-dir "$DEPLOY_DIR"
exasol start --deployment-dir "$DEPLOY_DIR"
```

### Optional: run PostgreSQL inside the macOS virtual machine

For a self-contained test, run PostgreSQL on a dedicated Podman network inside the managed virtual
machine and connect the Exasol container to that network.

Open the virtual-machine shell:

```bash
exasol shell host --deployment-dir "$DEPLOY_DIR"
```

Then run:

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

The dedicated network gives Exasol a route to PostgreSQL while keeping the containers independent.
Do not use `--network container:<Exasol-container>` because that dependency can prevent
`exasol destroy` from removing the deployment. The example writes PostgreSQL's address into the
shared staging directory because Podman DNS aliases are not reliable in every managed environment.

Read the address on the macOS host:

```bash
PG_HOST=$(tr -d '\r\n' < "$VS_DIR/postgres-ip")
```

Remove the test container with `podman rm --force exasol-vs-postgres` from `exasol shell host` when
it is no longer needed. Destroying the virtual machine also removes it.

### Create the connection and virtual schema

The PostgreSQL host must be reachable from the database container or managed virtual machine.
`localhost` refers to that environment, not necessarily to the computer running PostgreSQL. For an
external database, use an address reachable from the Exasol runtime. When PostgreSQL runs on a Mac
host, do not use `127.0.0.1`; publish it on a reachable host interface and verify connectivity from
`exasol shell host`.

Create `$DEPLOY_DIR/create-vs.sql` and replace the host, credentials, and source schema:

```sql
CREATE OR REPLACE CONNECTION POSTGRES_CONN
  TO 'jdbc:postgresql://192.168.1.20:5432/vs_test'
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
```

Run it against the selected deployment:

```bash
exasol connect --deployment-dir "$DEPLOY_DIR" -f "$DEPLOY_DIR/create-vs.sql"
```

Refresh the virtual schema after the source schema changes:

```sql
ALTER VIRTUAL SCHEMA VS_POSTGRES REFRESH;
```

## Other adapters

For another JDBC dialect, substitute the adapter JAR, driver JAR, and the `DRIVERNAME`, `PREFIX`,
`DRIVERMAIN`, and `JAR` settings. For a non-JDBC adapter, install its required SLC and follow the
adapter's dependency and script-definition instructions. The
[Exasol virtual schema documentation](https://docs.exasol.com/db/latest/database_concepts/virtual_schemas.htm)
lists available adapters and their properties.

## Troubleshooting

**`ETL-1014: No default DRIVER registered`**: Check that `/exa/jdbc/<DRIVERNAME>/` contains the JAR
and `settings.cfg`, that the configuration ends with an empty line, and that you restarted the
deployment.

**The adapter script cannot find its class**: Check that every `%jar` path resolves below `/buckets`
and that the files exist in the corresponding BucketFS directory.

**Adapter execution or `IMPORT` reports no runtime**: Run `exasol slc list` and confirm that the
adapter's SLC is installed. JDBC adapters require the Java SLC.
