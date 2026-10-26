## ADDED Requirements

### Requirement: Direct-host deployments use an installed engine
The system SHALL run a fresh Linux or Windows local deployment whose `container_engine` is `auto` through the first installed engine in the platform's order of preference, which is Podman then Docker on Linux and on Windows, and through the host Podman path when neither is installed.

#### Scenario: Podman and Docker are installed
- **WHEN** a user installs a fresh local deployment on Linux or Windows where both Podman and Docker are installed
- **THEN** the launcher prepares and uses host Podman

#### Scenario: Only Docker is installed
- **WHEN** a user installs a fresh local deployment on Linux or Windows where `docker` resolves to Docker and Podman is not installed
- **THEN** the launcher checks that Docker is ready and runs Nano in a Docker container named for the deployment

#### Scenario: Neither engine is installed
- **WHEN** a user installs a fresh local deployment on Linux or Windows where neither Podman nor Docker is installed
- **THEN** the launcher prepares and uses host Podman, including the Windows Podman installation approval

#### Scenario: Docker command runs Podman
- **WHEN** Podman is not installed under its own name and the resolved `docker` command is served by Podman
- **THEN** the deployment uses the host Podman path

#### Scenario: Docker runs only Windows containers
- **WHEN** a user installs a fresh local deployment on Windows with `container_engine` set to `auto`, Podman is not installed, and the Docker daemon reports that it runs Windows containers
- **THEN** the launcher prepares and uses host Podman

### Requirement: Users can select the container engine
The system SHALL offer a `container_engine` local configuration option on Linux and Windows with the values `auto`, `docker`, and `podman`, defaulting to `auto`, and SHALL run a deployment on an explicitly selected engine, failing with guidance that names that engine when it cannot be used.

#### Scenario: Podman is selected on a host with Docker
- **WHEN** a user runs `exasol install local --container-engine podman` on a Linux or Windows host where Docker is installed
- **THEN** Nano runs through host Podman

#### Scenario: Docker is selected
- **WHEN** a user runs `exasol init local --container-engine docker` and then `exasol deploy` on a host where Docker is usable
- **THEN** Nano runs in a Docker container named for the deployment

#### Scenario: Docker is selected before the first start
- **WHEN** a user runs `exasol config set --container-engine docker` for an initialized local deployment that has not started
- **THEN** the command succeeds and `exasol config get container_engine` prints `docker`

#### Scenario: Selected engine is unavailable
- **WHEN** a deployment's `container_engine` is `docker` and no Docker CLI resolves
- **THEN** deploy fails with guidance naming Docker

#### Scenario: Engine in use is inspected
- **WHEN** a user runs `exasol diag local` for a Linux or Windows local deployment
- **THEN** the output reports the container engine the deployment uses

#### Scenario: Default configuration is inspected
- **WHEN** a user runs `exasol config get container_engine` for a Linux or Windows local deployment initialized without the option
- **THEN** the command prints `auto`

#### Scenario: Empty engine is given
- **WHEN** a user runs `exasol config set --container-engine ''` for a Linux or Windows local deployment
- **THEN** the command fails, lists `auto`, `podman`, and `docker`, and leaves the configuration unchanged

### Requirement: A local deployment keeps its container engine
The system SHALL operate every direct-host local deployment through the container engine it was first prepared with until the deployment is destroyed, and SHALL treat existing deployments without a recorded engine as Podman deployments.

#### Scenario: Docker is installed after a Podman deployment was created
- **WHEN** a deployment was prepared with Podman and Docker is installed afterwards
- **THEN** `start`, `stop`, `status`, and `destroy` operate the deployment through Podman

#### Scenario: Deployment predates engine selection
- **WHEN** a direct-host deployment created by an earlier launcher has local runtime data and no recorded container engine
- **THEN** the launcher operates it through Podman

#### Scenario: Engine change is requested for a prepared deployment
- **WHEN** a user runs `exasol config set --container-engine docker` for a stopped deployment prepared with Podman
- **THEN** the command fails, leaves the configuration unchanged, and tells the user to destroy the deployment before changing its container engine and that destroying deletes its data

#### Scenario: Engine change is requested for a running deployment
- **WHEN** a user runs `exasol config set --container-engine docker` for a running deployment prepared with Podman
- **THEN** the command fails and tells the user to destroy the deployment before changing its container engine

#### Scenario: Destroyed deployment selects again
- **WHEN** a user destroys a deployment, sets a different `container_engine`, and deploys it again
- **THEN** Nano runs through the newly selected engine

#### Scenario: Recorded engine is unavailable
- **WHEN** a deployment's recorded container engine can no longer be resolved
- **THEN** the command fails with guidance that names that engine

### Requirement: Docker is used as found
The system SHALL use the user's Docker installation and CLI context as found, and SHALL explain why an installed Docker cannot be used.

#### Scenario: Docker daemon is not reachable
- **WHEN** a Docker deployment is deployed or started and the Docker daemon does not answer
- **THEN** the command fails and its message names the daemon state, socket permission, and CLI context as causes to check

#### Scenario: Docker context selects the daemon
- **WHEN** the user's Docker CLI context or `DOCKER_HOST` selects a daemon
- **THEN** the deployment's container runs on that daemon

#### Scenario: Docker Engine is older than 29
- **WHEN** a Docker deployment is deployed or started and the Docker daemon reports a Docker Engine version older than 29
- **THEN** the command fails, names the reported version and Docker Engine 29 as the minimum, and suggests selecting Podman

#### Scenario: Database port is in use
- **WHEN** a Docker deployment is deployed and another process holds its database port
- **THEN** the command fails and names the `db` service and that port

### Requirement: Docker deployments follow the Docker daemon
The system SHALL let Docker restart the database of a running deployment that has been stopped and started again with `exasol stop` and `exasol start` after installation when the Docker daemon starts again, and SHALL report a Docker daemon that does not answer as an unreachable container engine, keeping the deployment's recorded state.

#### Scenario: Docker daemon restarts after a start
- **WHEN** a user stops and starts a Docker deployment with `exasol stop` and `exasol start`, the database becomes ready, and the Docker daemon restarts
- **THEN** the database accepts connections again without a launcher command

#### Scenario: Status is requested while the daemon does not answer
- **WHEN** a Docker deployment's recorded state is running, the Docker daemon does not answer, and the user runs `exasol status`
- **THEN** the command reports status `database_connection_failed` with a message that the container engine is not answering, and its error names the daemon's reply

#### Scenario: Stop is requested while Docker restarts the database
- **WHEN** Docker is restarting a running deployment's database container and the user runs `exasol stop`
- **THEN** the command removes the database container and `exasol status` reports the deployment as stopped

#### Scenario: Stop is requested while the daemon does not answer
- **WHEN** a Docker deployment's recorded state is running, the Docker daemon does not answer, and the user runs `exasol stop`
- **THEN** the command fails with guidance naming the daemon state, socket permission, and CLI context, and `exasol info` still reports the deployment state as running

### Requirement: UDFs run under Docker
The system SHALL start Nano under Docker so that UDFs of installed official SLCs execute, whether Docker runs containers as root or rootless.

#### Scenario: Official SLC UDF runs under rootful Docker
- **WHEN** an official SLC is installed in a Docker deployment whose daemon runs containers as root, and a UDF of that language is executed
- **THEN** the UDF completes and returns its result

#### Scenario: Official SLC UDF runs under rootless Docker
- **WHEN** an official SLC is installed in a Docker deployment whose daemon runs rootless, and a UDF of that language is executed
- **THEN** the UDF completes and returns its result

#### Scenario: Official SLC UDF runs where AppArmor restricts user namespaces
- **WHEN** an official SLC is installed in a Docker deployment whose daemon runs containers as root and applies AppArmor, on a host where `kernel.apparmor_restrict_unprivileged_userns` is `1`, and a UDF of that language is executed
- **THEN** the UDF completes and returns its result

#### Scenario: UDF reads a staged BucketFS file
- **WHEN** the launcher user writes a file below `local/runtime/exa/bucketfs/bfsdefault/default` and a UDF reads it through `/buckets/bfsdefault/default`
- **THEN** the UDF returns the file's content

### Requirement: Docker deployment directories stay manageable by the launcher user
The system SHALL let the launcher user, with their own rights, stage BucketFS content and JDBC drivers and destroy a Docker deployment, including when Docker runs containers as root.

#### Scenario: User stages files in a fresh deployment
- **WHEN** a fresh Docker deployment is running, including one whose first start was interrupted and started again
- **THEN** the launcher user can create files and directories below `local/runtime/exa/bucketfs/bfsdefault/default` and `local/runtime/exa/jdbc`

#### Scenario: Deployment with database-owned files is destroyed
- **WHEN** a user runs `exasol destroy` for a Docker deployment whose data directory, or data set aside after an interrupted first start, contains files owned by root
- **THEN** the deployment's `local` directory and container are removed and the command succeeds when run as the launcher user

#### Scenario: Destroy fails before the data is removed
- **WHEN** `exasol destroy` fails for a Docker deployment before its data is removed
- **THEN** `exasol diag local` still reports `docker` as the deployment's engine, and a retried `exasol destroy` runs through Docker
