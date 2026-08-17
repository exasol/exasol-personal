# deployment-info-reporting Specification

## Purpose

Define how `exasol info` reports deployment state, next steps, and machine-readable deployment information.

## ADDED Requirements

### Requirement: Info SHALL report uninitialized deployment directories

`exasol info` SHALL succeed when the resolved deployment directory has no initialized deployment yet, and SHALL report that state to the user.

#### Scenario: Text output before initialization

- **WHEN** a user runs `exasol info` for a resolved deployment directory with no initialized deployment
- **THEN** the command succeeds
- **AND** the output identifies the resolved deployment directory
- **AND** the output reports the deployment state as `not_initialized`
- **AND** the output tells the user how to discover presets and create a deployment

#### Scenario: JSON output before initialization

- **WHEN** a user runs `exasol info --json` for a resolved deployment directory with no initialized deployment
- **THEN** the command succeeds
- **AND** stdout contains valid JSON
- **AND** the JSON contains the resolved deployment directory
- **AND** the JSON reports `deploymentState` as `not_initialized`

### Requirement: Info text output SHALL guide users by deployment state

Human-readable `exasol info` output SHALL include concise next-step guidance appropriate to the current deployment state.

#### Scenario: Initialized deployment

- **WHEN** a user runs `exasol info` for an initialized deployment that has not been deployed yet
- **THEN** the output reports the initialized state
- **AND** the output points the user toward deploying the database

#### Scenario: Running deployment

- **WHEN** a user runs `exasol info` for a running deployment
- **THEN** the output reports the running state
- **AND** the output points the user toward connecting to or stopping the deployment

#### Scenario: Stopped deployment

- **WHEN** a user runs `exasol info` for a stopped deployment
- **THEN** the output reports the stopped state
- **AND** the output points the user toward starting or destroying the deployment

#### Scenario: Deployment operation is not complete

- **WHEN** a user runs `exasol info` while a deployment operation is in progress, interrupted, or failed
- **THEN** the output reports the current state
- **AND** the output gives a state-appropriate next step

### Requirement: Info JSON output SHALL be machine-readable deployment information

`exasol info --json` SHALL write only valid JSON to stdout and SHALL provide a structured deployment overview suitable for scripts and agents.

#### Scenario: JSON stdout is parseable

- **WHEN** a user runs `exasol info --json`
- **THEN** stdout contains valid JSON
- **AND** stdout does not contain banners, prompts, or explanatory terminal prose outside JSON

#### Scenario: Initialized deployment metadata is available

- **WHEN** a user runs `exasol info --json` for an initialized deployment
- **THEN** the JSON includes the deployment state
- **AND** the JSON includes the resolved deployment directory
- **AND** the JSON includes the deployment identifier when one is available
- **AND** the JSON includes selected preset information when preset identity is available

#### Scenario: Running deployment connection details are available

- **WHEN** a user runs `exasol info --json` for a running deployment with connection details
- **THEN** the JSON includes connection-relevant fields such as backend, host, database port, username, TLS guidance, Admin UI details when available, and shell access details when available

#### Scenario: Connection details are not stable for the current state

- **WHEN** a user runs `exasol info --json` for a deployment state where connection details are absent or not stable
- **THEN** the JSON still reports the deployment state and resolved deployment directory
- **AND** connection details are omitted

### Requirement: Info SHALL preserve deployment lifecycle behavior

`exasol info` SHALL report deployment information without creating, modifying, starting, stopping, destroying, or removing deployment resources.

#### Scenario: Information command does not mutate deployment lifecycle

- **WHEN** a user runs `exasol info` or `exasol info --json`
- **THEN** the command reports the current deployment information
- **AND** the command does not change the deployment lifecycle state
