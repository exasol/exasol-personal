## MODIFIED Requirements

### Requirement: Local shell access

The CLI SHALL provide `exasol shell host` and `exasol shell container` for supported local deployments and SHALL report the shell-specific cause as the authoritative error when a requested shell cannot be opened.

#### Scenario: Host shell

- **WHEN** a macOS local deployment is running and the user runs `exasol shell host`
- **THEN** the command opens an interactive shell in the environment hosting the local deployment

#### Scenario: Container shell

- **WHEN** a macOS local deployment is running and the user runs `exasol shell container`
- **THEN** the command opens an interactive shell in the local deployment's database container environment

#### Scenario: Shell launch fails

- **WHEN** a supported local shell command cannot open the requested environment
- **THEN** the command fails and reports the shell-specific cause as the authoritative error

### Requirement: Linux local shell commands fail explicitly

The CLI SHALL report host and container shell commands as unsupported for Linux local deployments.

#### Scenario: Host shell requested on Linux

- **WHEN** a user runs `exasol shell host` for a Linux local deployment
- **THEN** the command fails with an explicit error identifying host shell access as unsupported on Linux

#### Scenario: Container shell requested on Linux

- **WHEN** a user runs `exasol shell container` for a Linux local deployment
- **THEN** the command fails with an explicit error identifying container shell access as unsupported on Linux
