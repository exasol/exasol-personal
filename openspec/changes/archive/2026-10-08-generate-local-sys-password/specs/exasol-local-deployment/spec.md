## MODIFIED Requirements

### Requirement: Local deployment artifacts

The system SHALL write standard launcher artifacts for local deployments so existing information, connection, status, and shell commands can operate on the deployment directory.

#### Scenario: Connection artifacts are written after startup

- **WHEN** the local Podman installation starts successfully
- **THEN** the launcher writes `deployment.json`, `secrets.json`, and connection instructions with loopback application connection details

#### Scenario: Local credentials are available

- **WHEN** a local deployment has started successfully
- **THEN** `secrets.json` contains the password the local database accepts for user `sys`

#### Scenario: Forwarded ports are refreshed

- **WHEN** a macOS local deployment is started after being stopped
- **THEN** the launcher refreshes `deployment.json` with the current labeled database and UI forwards without recording an SSH endpoint

## ADDED Requirements

### Requirement: Fresh local deployments use a generated database password

The system SHALL initialize the `sys` user of a fresh local deployment with a password generated for that deployment and SHALL keep that password in `secrets.json` across lifecycle commands and retries.

#### Scenario: Fresh deployment receives a generated password

- **WHEN** a user installs a fresh local deployment
- **THEN** `secrets.json` contains a password of at least 16 letters and digits that the database accepts for user `sys`, and the database rejects the password `exasol`

#### Scenario: Generated password survives stop and start

- **WHEN** a user stops and starts a local deployment that holds a generated password
- **THEN** `secrets.json` contains the same password, and `exasol connect` connects with it

#### Scenario: Retried first initialization keeps the stored password

- **WHEN** the first initialization of a local deployment fails and the user runs the deployment command again
- **THEN** the database accepts the password already stored in `secrets.json`

#### Scenario: Mismatched password fails with guidance

- **WHEN** the database rejects the stored password after first initialization
- **THEN** the deployment command fails with guidance naming the commands that recreate the deployment

#### Scenario: Bootstrap material is removed

- **WHEN** the first initialization of a local deployment succeeds, fails, or is interrupted
- **THEN** `secrets.json` is the only file in the deployment directory that contains the password

### Requirement: Existing local deployments keep their stored credentials

The system SHALL start local deployments initialized with `sys` / `exasol` with the credential already stored in their `secrets.json`, and SHALL require a missing generated credential to be restored or the deployment recreated.

#### Scenario: Existing deployment connects with its stored credential

- **WHEN** this launcher starts a local deployment whose database was initialized with `sys` / `exasol`
- **THEN** `exasol connect` connects with the password stored in `secrets.json`

#### Scenario: Missing secrets for a generated credential

- **WHEN** a user starts a local deployment that holds a generated credential and whose `secrets.json` is missing
- **THEN** the command fails with guidance to restore `secrets.json` or destroy and redeploy
