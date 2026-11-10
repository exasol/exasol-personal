## MODIFIED Requirements

### Requirement: Direct-host deployments use an installed engine
The system SHALL run a fresh Linux or Windows local deployment whose `container_engine` is `auto` through the first installed engine in the platform's order of preference, which is Podman then Docker on Linux and on Windows, through the host Podman path on Linux when neither is installed, and through the engine installed for it on Windows when neither is installed.

#### Scenario: Podman and Docker are installed
- **WHEN** a user installs a fresh local deployment on Linux or Windows where both Podman and Docker are installed
- **THEN** the launcher prepares and uses host Podman

#### Scenario: Only Docker is installed
- **WHEN** a user installs a fresh local deployment on Linux or Windows where `docker` resolves to Docker and Podman is not installed
- **THEN** the launcher checks that Docker is ready and runs Nano in a Docker container named for the deployment

#### Scenario: Neither engine is installed on Linux
- **WHEN** a user installs a fresh local deployment on Linux where neither Podman nor Docker is installed
- **THEN** the launcher prepares and uses host Podman

#### Scenario: Docker command runs Podman
- **WHEN** Podman is not installed under its own name and the resolved `docker` command is served by Podman
- **THEN** the deployment uses the host Podman path

#### Scenario: Docker runs only Windows containers
- **WHEN** a user installs a fresh local deployment on Windows with `container_engine` set to `auto`, Podman is not installed, and the Docker daemon reports that it runs Windows containers
- **THEN** the launcher prepares and uses host Podman

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
- **THEN** the command fails, names the reported version and Docker Engine 29 as the minimum, and suggests selecting Podman when the deployment has not recorded its engine

#### Scenario: Docker command reaches Podman
- **WHEN** a Docker deployment is deployed or started and the `docker` command reaches a Podman daemon
- **THEN** the command fails, names Podman and its version, and suggests pointing the `docker` command at a Docker daemon, and selecting Podman when the deployment has not recorded its engine

#### Scenario: Database port is in use
- **WHEN** a Docker deployment is deployed and another process holds its database port
- **THEN** the command fails and names the `db` service and that port

## ADDED Requirements

### Requirement: Windows hosts install the engine a deployment needs
The system SHALL, on Windows, install the container engine a new local deployment needs through Windows Package Manager after approval, SHALL let an interactive run choose between Podman and Docker Desktop in one prompt when `container_engine` is `auto` and neither engine is installed, SHALL save the chosen engine as the deployment's `container_engine`, and SHALL start a Docker Desktop it installed and wait up to 10 minutes for it to run Linux containers.

#### Scenario: User chooses Docker
- **WHEN** a user runs `exasol install local` in a terminal on Windows where neither engine is installed and chooses Docker Desktop
- **THEN** Docker Desktop is installed and started, Nano runs in a Docker container named for the deployment, and `exasol config get container_engine` prints `docker`

#### Scenario: User chooses Podman
- **WHEN** a user runs `exasol install local` in a terminal on Windows where neither engine is installed and chooses Podman
- **THEN** Podman is installed, Nano runs through host Podman, and `exasol config get container_engine` prints `podman`

#### Scenario: Docker is selected explicitly
- **WHEN** a user runs `exasol install local --container-engine docker` in a terminal on Windows where Docker is not installed and approves the installation
- **THEN** Docker Desktop is installed and started, and Nano runs in a Docker container named for the deployment

#### Scenario: Installation is approved in advance
- **WHEN** a user runs `exasol install local --auto-approve` on Windows where neither engine is installed
- **THEN** Podman is installed and Nano runs through host Podman

#### Scenario: Run without a terminal
- **WHEN** a user runs `exasol install local` without a terminal and without `--auto-approve` on Windows where neither engine is installed
- **THEN** the command fails and names `--auto-approve` and `exasol config set --container-engine`

#### Scenario: Docker Desktop does not become ready
- **WHEN** the launcher installed Docker Desktop and its daemon does not run Linux containers within 10 minutes
- **THEN** the command fails and tells the user to finish starting Docker Desktop and re-run the command, and the re-run deploys on Docker

#### Scenario: Installation prompt names the other engine
- **WHEN** the launcher asks on Windows to install the engine of a deployment that has not recorded its engine, and the other engine is usable or not installed
- **THEN** the prompt names the `exasol config set --container-engine` command that selects the other engine

#### Scenario: Other engine cannot run
- **WHEN** the launcher asks on Windows to install Podman for an `auto` deployment whose Docker runs Windows containers
- **THEN** the prompt offers only the Podman installation

#### Scenario: Podman answers in Docker Desktop's place
- **WHEN** the launcher installed Docker Desktop and the `docker` command reaches Podman on Docker's default pipe
- **THEN** the command fails without waiting, names Podman as what stops Docker Desktop from starting, and tells the user to stop the Podman machine and re-run the command

#### Scenario: Docker command is pointed at Podman
- **WHEN** the launcher installed Docker Desktop and `DOCKER_HOST` or the current Docker context points the `docker` command at Podman
- **THEN** the command fails without waiting, names the endpoint and what selects it, and tells the user to unset `DOCKER_HOST` or switch to Docker Desktop's context

#### Scenario: Docker was installed after the terminal opened
- **WHEN** Docker Desktop was installed after the terminal running the launcher was opened
- **THEN** a deployment whose `container_engine` is `docker` runs through Docker
