# launcher-version-check Specification

## Purpose
Defines launcher update-check behavior, including semantic version ordering, prerelease handling, explicit version command output, and implicit update hint stream routing.

## Requirements
### Requirement: Automatic Launcher Update Hint Uses Semantic Ordering
The launcher SHALL show an automatic update hint only when the version-check service reports a launcher version with greater semantic version precedence than the current launcher version.

#### Scenario: Older official release is not an update for newer release candidate
- **WHEN** the current launcher version is `2.0.0-rc1`
- **AND** the version-check service reports `1.4.1`
- **THEN** the launcher does not show an automatic update hint

#### Scenario: Newer patch release is an update
- **WHEN** the current launcher version is `1.4.0`
- **AND** the version-check service reports `1.4.1`
- **THEN** the launcher shows an automatic update hint for `1.4.1`

#### Scenario: Equal versions are not an update
- **WHEN** the current launcher version is `1.4.1`
- **AND** the version-check service reports `1.4.1`
- **THEN** the launcher does not show an automatic update hint

### Requirement: Prerelease Versions Follow SemVer Precedence
The launcher SHALL compare prerelease versions according to semantic version precedence.

#### Scenario: Final release is newer than its release candidate
- **WHEN** the current launcher version is `2.0.0-rc1`
- **AND** the version-check service reports `2.0.0`
- **THEN** the launcher treats `2.0.0` as an available update

#### Scenario: Release candidate is newer than older final release
- **WHEN** the current launcher version is `1.4.1`
- **AND** the version-check service reports `2.0.0-rc1`
- **THEN** the launcher treats `2.0.0-rc1` as an available update

### Requirement: Explicit Latest Version Output Is Accurate
The `exasol version --latest` command SHALL distinguish newer, equal, and older reported versions in its user-facing text.

#### Scenario: Reported version is newer
- **WHEN** the current launcher version is `1.4.0`
- **AND** the version-check service reports `1.4.1`
- **THEN** the command says a newer version is available

#### Scenario: Reported version is equal
- **WHEN** the current launcher version is `1.4.1`
- **AND** the version-check service reports `1.4.1`
- **THEN** the command says the user is using the latest version

#### Scenario: Reported version is older
- **WHEN** the current launcher version is `2.0.0-rc1`
- **AND** the version-check service reports `1.4.1`
- **THEN** the command says the reported latest official version is not newer than the current launcher version

### Requirement: Invalid Version Data Does Not Produce Update Hints
The launcher SHALL NOT produce an automatic update hint when either the current launcher version or the reported latest version cannot be parsed as a semantic version.

#### Scenario: Reported version is invalid
- **WHEN** the version-check service reports an invalid version string
- **THEN** the launcher does not show an automatic update hint
- **AND** the check is handled as a best-effort failure

### Requirement: Version Check Output Streams Preserve Primary Command Output
The launcher SHALL write explicit version command output to stdout and implicit update metadata to stderr.

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

#### Scenario: Implicit update hint is metadata
- **WHEN** a command other than `exasol version` discovers an available launcher update through the automatic update check
- **THEN** the update hint is written as a terminal notice on stderr
- **AND** the command's stdout remains reserved for primary command output
