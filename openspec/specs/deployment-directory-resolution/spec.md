# deployment-directory-resolution Specification

## Purpose
Define how Exasol Personal resolves the active deployment directory so users can run deployment commands without first creating or changing into a directory, while preserving explicit overrides and existing deployment-directory workflows.

## Requirements
### Requirement: CLI SHALL resolve an active deployment directory
Commands that operate on a deployment directory SHALL resolve exactly one active deployment directory before command-specific execution.

#### Scenario: Explicit deployment directory wins
- **WHEN** a user invokes a deployment command with `--deployment-dir <path>`
- **THEN** the CLI uses `<path>` as the active deployment directory
- **AND** the CLI does not use the current working directory or default deployment directory for that command

#### Scenario: Recognized current deployment directory wins when no flag is provided
- **WHEN** a user invokes a deployment command without `--deployment-dir`
- **AND** the current working directory contains launcher-owned deployment state or marker files
- **THEN** the CLI uses the current working directory as the active deployment directory

#### Scenario: Default deployment directory is used when no stronger source exists
- **WHEN** a user invokes a deployment command without `--deployment-dir`
- **AND** the current working directory is not recognized as a deployment directory
- **THEN** the CLI uses `~/.exasol/personal/deployments/default` as the active deployment directory

### Requirement: CLI SHALL keep the default deployment directory visible
Commands that resolve to the default deployment directory SHALL emit a clear human-facing message containing the resolved path.

#### Scenario: Default directory message is shown
- **WHEN** a deployment command resolves to the default deployment directory
- **THEN** the user sees a message containing the resolved default deployment directory path

#### Scenario: JSON payload output remains parseable
- **WHEN** a deployment command produces JSON output and resolves to the default deployment directory
- **THEN** the command's stdout remains valid JSON
- **AND** any human-facing default-directory message is emitted outside the JSON payload

### Requirement: Init and install SHALL create the default deployment directory when needed
Commands that initialize a deployment SHALL automatically create the default deployment directory when resolution selects it and it does not already exist.

#### Scenario: Install creates the default deployment directory
- **WHEN** a user runs `exasol install <preset>` outside a recognized deployment directory without `--deployment-dir`
- **THEN** the CLI creates `~/.exasol/personal/deployments/default`
- **AND** the install proceeds using that directory

#### Scenario: Init creates the default deployment directory
- **WHEN** a user runs `exasol init <preset>` outside a recognized deployment directory without `--deployment-dir`
- **THEN** the CLI creates `~/.exasol/personal/deployments/default`
- **AND** initialization proceeds using that directory

### Requirement: Commands that require initialized state SHALL fail clearly for uninitialized directories
Deployment commands that require an initialized deployment SHALL fail when the resolved active deployment directory is not initialized, and the error SHALL include the resolved path.

#### Scenario: Required initialized command uses default directory but it is uninitialized
- **WHEN** a user runs a command that requires initialized deployment state outside a recognized deployment directory without `--deployment-dir`
- **AND** the default deployment directory is not initialized
- **THEN** the command fails with a message that says the deployment directory is not initialized
- **AND** the message includes the resolved default deployment directory path

#### Scenario: Required initialized command uses explicit directory but it is uninitialized
- **WHEN** a user runs a command that requires initialized deployment state with `--deployment-dir <path>`
- **AND** `<path>` is not initialized
- **THEN** the command fails with a message that says the deployment directory is not initialized
- **AND** the message includes `<path>`

### Requirement: Status SHALL report the active deployment directory
The `status` command SHALL include the active deployment directory path in both text and JSON output.

#### Scenario: Status reports explicit deployment directory
- **WHEN** a user runs `exasol status --deployment-dir <path>`
- **THEN** the status output includes `<path>` as the active deployment directory

#### Scenario: Status reports current deployment directory
- **WHEN** a user runs `exasol status` from a recognized deployment directory
- **THEN** the status output includes the current working directory as the active deployment directory

#### Scenario: Status reports default deployment directory
- **WHEN** a user runs `exasol status` outside a recognized deployment directory without `--deployment-dir`
- **THEN** the status output includes `~/.exasol/personal/deployments/default` as the active deployment directory

### Requirement: Status SHALL report uninitialized resolved directories
The `status` command SHALL report `not_initialized` for an uninitialized resolved deployment directory instead of failing only because initialization has not happened.

#### Scenario: Status reports uninitialized default directory
- **WHEN** a user runs `exasol status` outside a recognized deployment directory without `--deployment-dir`
- **AND** the default deployment directory is not initialized
- **THEN** the command succeeds
- **AND** the status is `not_initialized`
- **AND** the output includes the resolved default deployment directory path

#### Scenario: Status reports uninitialized explicit directory
- **WHEN** a user runs `exasol status --deployment-dir <path>`
- **AND** `<path>` is not initialized
- **THEN** the command succeeds
- **AND** the status is `not_initialized`
- **AND** the output includes `<path>` as the active deployment directory

### Requirement: Documentation SHALL explain deployment directory behavior
User-facing help and documentation SHALL explain the default deployment directory, precedence rules, override behavior, and cloud cleanup implications.

#### Scenario: Help describes how to override the active deployment directory
- **WHEN** a user reads command help for a deployment command
- **THEN** the help explains that `--deployment-dir` overrides automatic deployment directory resolution

#### Scenario: Documentation warns about preserving deployment directories for cloud cleanup
- **WHEN** a user reads deployment documentation
- **THEN** the documentation explains that cloud deployment directories should be kept until cloud resources are destroyed

