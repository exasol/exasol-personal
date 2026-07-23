# launcher-version-check Specification

## MODIFIED Requirements

### Requirement: Version Check Output Streams Preserve Primary Command Output

The launcher SHALL write explicit version command output to stdout and implicit update metadata to stderr. The implicit update hint is call-to-action guidance written to stderr; the launcher SHALL suppress it only when the invoking command produces JSON output, and SHALL NOT gate it on an interactive terminal.

#### Scenario: Explicit current-version JSON report is primary output

- **WHEN** the user runs `exasol version --json`
- **THEN** the current launcher version is written as JSON to stdout
- **AND** the response is routed through the terminal output queue

#### Scenario: Explicit text latest-version report is primary output

- **WHEN** the user runs `exasol version --latest`
- **THEN** the human-readable latest-version report is written to stdout
- **AND** the report is routed through the terminal output queue

#### Scenario: Explicit JSON latest-version report is primary output

- **WHEN** the user runs `exasol version --latest --json`
- **THEN** the JSON latest-version response is written to stdout
- **AND** the response is routed through the terminal output queue
- **AND** the JSON response is not mixed with human-readable update text

#### Scenario: Implicit update hint is shown for text output

- **WHEN** a command other than `exasol version` discovers an available launcher update through the automatic update check
- **AND** the command is not producing JSON output
- **THEN** the update hint is written as a call to action on stderr, whether or not standard error is an interactive terminal
- **AND** the command's stdout remains reserved for primary command output

#### Scenario: Implicit update hint is suppressed under JSON

- **WHEN** a command other than `exasol version` discovers an available launcher update
- **AND** the command is producing JSON output
- **THEN** the launcher does not emit the update hint on any stream
