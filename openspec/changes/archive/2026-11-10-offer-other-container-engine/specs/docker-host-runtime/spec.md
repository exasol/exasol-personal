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
- **THEN** the launcher states that Docker runs Windows containers, and prepares and uses host Podman

## ADDED Requirements

### Requirement: An unusable automatic engine choice can be replaced before the first start
The system SHALL, when the engine `auto` chose for a local deployment without a recorded engine fails its readiness check and the other engine can run on the host, state why in an interactive run and offer the other engine, including its installation on Windows when it is missing, and SHALL save the accepted engine as the deployment's `container_engine` and run the deployment on it.

#### Scenario: User accepts the other engine
- **WHEN** a user deploys interactively, the engine `auto` chose fails its readiness check, and the user accepts the offered engine
- **THEN** Nano runs through the offered engine, and `exasol config get container_engine` prints that engine

#### Scenario: Other engine is missing on Windows
- **WHEN** the engine `auto` chose on Windows fails its readiness check and the other engine is not installed
- **THEN** the offer lists the other engine's installation, and accepting it installs that engine and runs Nano through it

#### Scenario: User declines the other engine
- **WHEN** a user declines the offered engine
- **THEN** the command fails with the chosen engine's error and the `exasol config set --container-engine` command that selects the other engine

#### Scenario: Run without a person to answer
- **WHEN** the engine `auto` chose fails its readiness check in a run with `--auto-approve` or without a terminal, and the other engine is usable or not installed
- **THEN** the command fails with the chosen engine's error and the `exasol config set --container-engine` command that selects the other engine

#### Scenario: Other engine cannot run either
- **WHEN** the engine `auto` chose fails its readiness check and the other engine is installed but also fails its readiness check
- **THEN** the command fails with the chosen engine's error alone
