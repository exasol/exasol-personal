## ADDED Requirements

### Requirement: A deployment whose engine was uninstalled can be destroyed
The system SHALL, when `exasol destroy` runs in a terminal for a direct-host local deployment whose container engine command is not installed, ask whether to destroy the deployment without the engine, naming the commands that remove the containers and network the engine may still hold, and on confirmation SHALL delete the deployment's data and runtime files and return it to its initialized state.

#### Scenario: User confirms the destroy
- **WHEN** a user runs `exasol destroy` in a terminal for a Docker deployment whose `docker` command is not installed and confirms the prompt
- **THEN** the prompt showed the `docker rm` and `docker network rm` commands for the deployment's container and network, the deployment's `local/runtime` directory is removed, and the command succeeds

#### Scenario: Destroy without a person to confirm
- **WHEN** `exasol destroy` runs with `--auto-approve` or without a terminal for a deployment whose container engine command is not installed
- **THEN** the command fails with the engine's error and the deployment's data remains
