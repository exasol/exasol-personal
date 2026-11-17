## MODIFIED Requirements

### Requirement: Direct-host deployments use an installed engine
The system SHALL run a fresh Linux or Windows local deployment whose `container_engine` is `auto` through the first installed engine in the platform's order of preference, which is Docker then Podman on Linux and on Windows, through the host Podman path on Linux when neither is installed, and through the engine installed for it on Windows when neither is installed.

#### Scenario: Podman and Docker are installed
- **WHEN** a user installs a fresh local deployment on Linux or Windows where both Podman and Docker are installed and Docker runs Linux containers
- **THEN** the launcher checks that Docker is ready and runs Nano in a Docker container named for the deployment

#### Scenario: Only Docker is installed
- **WHEN** a user installs a fresh local deployment on Linux or Windows where `docker` resolves to Docker and Podman is not installed
- **THEN** the launcher checks that Docker is ready and runs Nano in a Docker container named for the deployment

#### Scenario: Only Podman is installed
- **WHEN** a user installs a fresh local deployment on Linux or Windows where Podman is installed and no Docker CLI resolves
- **THEN** the launcher prepares and uses host Podman

#### Scenario: Neither engine is installed on Linux
- **WHEN** a user installs a fresh local deployment on Linux where neither Podman nor Docker is installed
- **THEN** the launcher prepares and uses host Podman

#### Scenario: Docker command runs Podman
- **WHEN** Podman is not installed under its own name and the resolved `docker` command is served by Podman
- **THEN** the deployment uses the host Podman path

#### Scenario: Docker runs only Windows containers
- **WHEN** a user installs a fresh local deployment on Windows with `container_engine` set to `auto`, Podman is not installed, and the Docker daemon reports that it runs Windows containers
- **THEN** the launcher states that Docker runs Windows containers, and prepares and uses host Podman
