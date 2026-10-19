## ADDED Requirements

### Requirement: Direct-host deployments prefer an installed Docker
The system SHALL run a fresh Linux or Windows local deployment whose `container_engine` is `auto` through host Docker when a Docker CLI resolves on PATH, and through the host Podman path otherwise.

#### Scenario: Docker is installed
- **WHEN** a user installs a fresh local deployment on Linux or Windows and `docker` resolves to Docker
- **THEN** Nano runs in a Docker container named for the deployment, and the launcher requests no host-change approval

#### Scenario: Docker is not installed
- **WHEN** a user installs a fresh local deployment on Linux or Windows and no Docker CLI resolves
- **THEN** the launcher prepares and uses host Podman, including the Windows Podman installation approval

#### Scenario: Docker command runs Podman
- **WHEN** the resolved `docker` command is served by Podman
- **THEN** the deployment uses the host Podman path

#### Scenario: Docker runs only Windows containers
- **WHEN** a user installs a fresh local deployment on Windows with `container_engine` set to `auto` and the Docker daemon reports that it runs Windows containers
- **THEN** the launcher prepares and uses host Podman

### Requirement: Users can select the container engine
The system SHALL offer a `container_engine` local configuration option on Linux and Windows with the values `auto`, `docker`, and `podman`, defaulting to `auto`, and SHALL use an explicitly selected engine without falling back to the other.

#### Scenario: Podman is selected on a host with Docker
- **WHEN** a user runs `exasol install local --container-engine podman` on a Linux or Windows host where Docker is installed
- **THEN** Nano runs through host Podman

#### Scenario: Docker is selected
- **WHEN** a user runs `exasol init local --container-engine docker` and then `exasol deploy` on a host where Docker is usable
- **THEN** Nano runs in a Docker container named for the deployment

#### Scenario: Selected engine is unavailable
- **WHEN** a deployment's `container_engine` is `docker` and no Docker CLI resolves
- **THEN** deploy fails with guidance naming Docker and does not prepare Podman

#### Scenario: Default configuration is inspected
- **WHEN** a user runs `exasol config get container_engine` for a Linux or Windows local deployment initialized without the option
- **THEN** the command prints `auto`

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
- **THEN** the command fails, leaves the configuration unchanged, and tells the user to destroy the deployment before changing its container engine

#### Scenario: Destroyed deployment selects again
- **WHEN** a user destroys a deployment, sets a different `container_engine`, and deploys it again
- **THEN** Nano runs through the newly selected engine

#### Scenario: Recorded engine is unavailable
- **WHEN** a deployment's recorded container engine can no longer be resolved
- **THEN** the command fails with guidance that names that engine, without switching to the other engine

### Requirement: Docker is used as found
The system SHALL use the user's Docker installation and CLI context without installing, configuring, or starting Docker, and SHALL explain why an installed Docker cannot be used.

#### Scenario: Docker daemon is not reachable
- **WHEN** a Docker deployment is deployed or started and the Docker daemon does not answer
- **THEN** the command fails and its message names the daemon state, socket permission, and CLI context as causes to check

#### Scenario: Docker context selects the daemon
- **WHEN** the user's Docker CLI context or `DOCKER_HOST` selects a daemon
- **THEN** the deployment's container runs on that daemon

### Requirement: UDFs run under Docker
The system SHALL start Nano under Docker so that UDFs of installed official SLCs execute, whether Docker runs containers as root or rootless.

#### Scenario: Official SLC UDF runs under rootful Docker
- **WHEN** an official SLC is installed in a Docker deployment whose daemon runs containers as root, and a UDF of that language is executed
- **THEN** the UDF completes and returns its result

#### Scenario: Official SLC UDF runs under rootless Docker
- **WHEN** an official SLC is installed in a Docker deployment whose daemon runs rootless, and a UDF of that language is executed
- **THEN** the UDF completes and returns its result

#### Scenario: UDF reads a staged BucketFS file
- **WHEN** the launcher user writes a file below `local/runtime/exa/bucketfs/bfsdefault/default` and a UDF reads it through `/buckets/bfsdefault/default`
- **THEN** the UDF returns the file's content

### Requirement: Docker deployment directories stay manageable by the launcher user
The system SHALL let the launcher user stage BucketFS content and JDBC drivers and destroy a Docker deployment without elevated rights, including when Docker runs containers as root.

#### Scenario: User stages files in a fresh deployment
- **WHEN** a fresh Docker deployment is running
- **THEN** the launcher user can create files and directories below `local/runtime/exa/bucketfs/bfsdefault/default` and `local/runtime/exa/jdbc`

#### Scenario: Deployment with database-owned files is destroyed
- **WHEN** a user runs `exasol destroy` for a Docker deployment whose data directory contains files owned by root
- **THEN** the deployment's `local` directory and container are removed and the command succeeds without elevated rights
