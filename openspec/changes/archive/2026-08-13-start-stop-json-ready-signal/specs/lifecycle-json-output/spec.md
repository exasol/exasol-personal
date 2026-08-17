## ADDED Requirements

### Requirement: Lifecycle commands emit machine-readable completion signals

`exasol start --json` and `exasol stop --json` SHALL emit exactly one JSON document to stdout after successful completion.

#### Scenario: Start emits running ready signal

- **WHEN** a stopped deployment is successfully started with `exasol start --json`
- **THEN** stdout contains exactly one JSON document
- **AND** the document includes `deploymentState` with value `running`
- **AND** the document includes `databaseReady` with value `true`

#### Scenario: Stop emits stopped ready signal

- **WHEN** a running deployment is successfully stopped with `exasol stop --json`
- **THEN** stdout contains exactly one JSON document
- **AND** the document includes `deploymentState` with value `stopped`
- **AND** the document includes `databaseReady` with value `false`

### Requirement: Lifecycle JSON output keeps stdout machine-readable

When lifecycle JSON mode is selected, command stdout SHALL contain only the lifecycle completion JSON document.

#### Scenario: Logs do not corrupt lifecycle JSON stdout

- **WHEN** `exasol start --json` or `exasol stop --json` runs with normal lifecycle logging
- **THEN** human-readable logs are not written to stdout
- **AND** stdout remains parseable as a single JSON document

#### Scenario: Start connection instructions are omitted from JSON stdout

- **WHEN** `exasol start --json` completes successfully
- **THEN** refreshed connection instructions are not written to stdout
- **AND** stdout contains only the lifecycle completion JSON document
