# credential-confidentiality Specification

## Purpose
Keeps database passwords confined to the deployment's secrets file, so that launcher output and the logs users share for troubleshooting describe database connections without revealing credentials.

## Requirements

### Requirement: Connection log records describe connections without passwords

The system SHALL describe database connections in log records by user, host, port, and certificate validation mode, on every deployment type and at every log level.

#### Scenario: Debug logging of a connection

- **WHEN** a user runs `exasol connect --log-level debug` with a password supplied through `--password`
- **THEN** stderr contains the connection's user, host, and port, and contains the supplied password nowhere

#### Scenario: Deployment log after a lifecycle command

- **WHEN** a lifecycle command that connects to the database with the stored password writes to `deployment.log`
- **THEN** `deployment.log` identifies the connection target, and the stored password appears only in `secrets.json`
