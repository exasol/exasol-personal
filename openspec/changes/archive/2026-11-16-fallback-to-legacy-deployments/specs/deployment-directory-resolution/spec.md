## MODIFIED Requirements

### Requirement: CLI SHALL resolve an active deployment directory
Commands that operate on a deployment directory SHALL resolve exactly one
active deployment directory before command-specific execution. For named and
implicit default selection, the CLI SHALL use the new managed-deployments
location when its path exists, the matching legacy location when only that
path exists, and the new location when neither path exists.

#### Scenario: Explicit deployment directory wins
- **WHEN** a user invokes a deployment command with
  `--deployment-dir <path>`
- **THEN** the CLI uses `<path>` as the active deployment directory
- **AND** the CLI does not use `--deployment`, the current working directory,
  or the default deployment directory for that command

#### Scenario: Explicit named deployment directory wins
- **WHEN** a user invokes a deployment command with `--deployment <name>`
  and without `--deployment-dir`
- **THEN** the CLI selects `<name>` from the new or legacy
  managed-deployments location according to the location preference above
- **AND** the CLI does not use the current working directory or the default
  deployment directory for that command, even if the current working
  directory is a recognized deployment directory

#### Scenario: Named selection finds a kept legacy deployment
- **WHEN** startup migration keeps `<name>` at the legacy location
- **AND** no path named `<name>` exists at the new location
- **AND** a user selects `--deployment <name>`
- **THEN** the command uses the legacy deployment directory

#### Scenario: Named selection prefers the new location on collision
- **WHEN** paths named `<name>` exist at both managed-deployments locations
- **AND** a user selects `--deployment <name>`
- **THEN** the command uses the path at the new location

#### Scenario: Recognized current deployment directory wins when no flag is provided
- **WHEN** a user invokes a deployment command without `--deployment-dir`
  and without `--deployment`
- **AND** the current working directory contains launcher-owned deployment
  state or marker files
- **THEN** the CLI uses the current working directory as the active deployment
  directory

#### Scenario: Default deployment directory is used when no stronger source exists
- **WHEN** a user invokes a deployment command without `--deployment-dir`
  and without `--deployment`
- **AND** the current working directory is not recognized as a deployment
  directory
- **THEN** the CLI selects `default` according to the same location
  preference as a named deployment

#### Scenario: Implicit default selection finds a kept legacy deployment
- **WHEN** startup migration keeps `default` at the legacy location
- **AND** no path named `default` exists at the new location
- **AND** a user invokes a deployment command outside a recognized deployment
  directory without a deployment selection flag
- **THEN** the command uses the legacy `default` deployment directory

### Requirement: CLI SHALL keep the resolved deployment directory visible
Commands that resolve to the default deployment directory, to a named
deployment directory, or to a recognized current working directory SHALL emit
one clear human-facing message containing the resolved path on standard error
before command-specific execution begins. The message SHALL warn when named or
implicit default selection uses a legacy deployment directory.

#### Scenario: Default directory message is shown early
- **WHEN** a deployment command resolves to the default deployment directory
- **THEN** the user sees a message on standard error containing the resolved
  default deployment directory path before command-specific execution begins
- **AND** the message does not appear again after command-specific execution
  ends

#### Scenario: Named directory message is shown early
- **WHEN** a deployment command resolves to a named deployment directory via
  `--deployment <name>`
- **THEN** the user sees a message on standard error containing `<name>` and
  the resolved path before command-specific execution begins
- **AND** the message does not appear again after command-specific execution
  ends

#### Scenario: Legacy selection warns before command execution
- **WHEN** named or implicit default selection uses a deployment directory at
  the legacy location
- **THEN** its resolved-path message on standard error warns that the selected
  deployment is at the legacy location
- **AND** the warning names the selected path before command-specific
  execution begins

#### Scenario: Current directory message is shown early
- **WHEN** a deployment command resolves to the current working directory
  because it is a recognized deployment directory
- **THEN** the user sees a message on standard error containing the current
  working directory path before command-specific execution begins
- **AND** the message does not appear again after command-specific execution
  ends

#### Scenario: JSON payload output remains parseable
- **WHEN** a deployment command produces JSON output and resolves to the
  default deployment directory, a named deployment directory, or the current
  working directory
- **THEN** the command's stdout remains valid JSON
- **AND** the human-facing resolved-directory message is emitted on standard
  error, not stdout

### Requirement: CLI SHALL reject ambiguous or invalid deployment directory selection
The CLI SHALL reject invocations that cannot resolve to exactly one unambiguous
deployment directory selection.

#### Scenario: --deployment-dir and --deployment are mutually exclusive
- **WHEN** a user invokes a deployment command with both
  `--deployment-dir <path>` and `--deployment <name>`
- **THEN** the CLI fails with a usage error
- **AND** the error identifies both flags as mutually exclusive
- **AND** the failure occurs before any deployment-directory side effect,
  including resolution, compatibility enforcement, deployment file logging,
  and version-update checks

#### Scenario: Named deployment directory rejects unsafe characters
- **WHEN** a user invokes a deployment command with `--deployment <name>`
  where `<name>` contains any character other than letters, digits, `-`, or `_`
- **THEN** the CLI fails with a usage error before command-specific execution
- **AND** the error explains which characters are allowed

#### Scenario: The literal name "default" is accepted
- **WHEN** a user invokes a deployment command with `--deployment default`
- **THEN** the CLI selects `default` using the same location preference as
  implicit default selection
- **AND** the CLI does not treat this as an error or as a distinct directory
  from the implicit default

### Requirement: Init and install SHALL create the default deployment directory when needed
Commands that initialize a deployment SHALL create its directory under the new
managed-deployments root when neither its new nor legacy path exists.

#### Scenario: Install creates the default deployment directory
- **WHEN** a user runs `exasol install <preset>` outside a recognized
  deployment directory without `--deployment-dir` and without `--deployment`
- **AND** neither `default` path exists
- **THEN** the CLI creates the `default` directory under the new
  managed-deployments root
- **AND** the install proceeds using that directory

#### Scenario: Init creates the default deployment directory
- **WHEN** a user runs `exasol init <preset>` outside a recognized deployment
  directory without `--deployment-dir` and without `--deployment`
- **AND** neither `default` path exists
- **THEN** the CLI creates the `default` directory under the new
  managed-deployments root
- **AND** initialization proceeds using that directory

#### Scenario: Install creates a named deployment directory
- **WHEN** a user runs `exasol install <preset> --deployment <name>`
- **AND** neither path named `<name>` exists
- **THEN** the CLI creates `<name>` under the new managed-deployments root
- **AND** the install proceeds using that directory

### Requirement: Status SHALL report the active deployment directory
The `status` command SHALL include the active deployment directory path in
both text and JSON output.

#### Scenario: Status reports explicit deployment directory
- **WHEN** a user runs `exasol status --deployment-dir <path>`
- **THEN** the status output includes `<path>` as the active deployment
  directory

#### Scenario: Status reports named deployment directory
- **WHEN** a user runs `exasol status --deployment <name>`
- **THEN** the status output includes the resolved `<name>` path as the active
  deployment directory

#### Scenario: Status reports current deployment directory
- **WHEN** a user runs `exasol status` from a recognized deployment directory
- **THEN** the status output includes the current working directory as the
  active deployment directory

#### Scenario: Status reports default deployment directory
- **WHEN** a user runs `exasol status` outside a recognized deployment
  directory without `--deployment-dir` and without `--deployment`
- **THEN** the status output includes the resolved `default` path as the active
  deployment directory

### Requirement: Status SHALL report uninitialized resolved directories
The `status` command SHALL report `not_initialized` for an uninitialized
resolved deployment directory instead of failing only because initialization
has not happened.

#### Scenario: Status reports uninitialized default directory
- **WHEN** a user runs `exasol status` outside a recognized deployment
  directory without `--deployment-dir` and without `--deployment`
- **AND** the resolved default deployment directory is not initialized
- **THEN** the command succeeds
- **AND** the status is `not_initialized`
- **AND** the output includes the resolved default deployment directory path

#### Scenario: Status reports uninitialized explicit directory
- **WHEN** a user runs `exasol status --deployment-dir <path>`
- **AND** `<path>` is not initialized
- **THEN** the command succeeds
- **AND** the status is `not_initialized`
- **AND** the output includes `<path>` as the active deployment directory

#### Scenario: Status reports uninitialized named directory
- **WHEN** a user runs `exasol status --deployment <name>`
- **AND** the resolved `<name>` directory is not initialized
- **THEN** the command succeeds
- **AND** the status is `not_initialized`
- **AND** the output includes the resolved named deployment directory path

### Requirement: Documentation SHALL explain deployment directory behavior
User-facing help SHALL explain the default deployment directory, named
deployment directories, and override behavior. Deployment documentation SHALL
explain location precedence, legacy fallback, and cloud cleanup implications.

#### Scenario: Help describes how to override the active deployment directory
- **WHEN** a user reads command help for a deployment command
- **THEN** the help explains that `--deployment-dir` overrides automatic
  deployment directory resolution
- **AND** the help explains that `--deployment <name>` (or `-d <name>`)
  selects a managed deployment by name as an alternative to `--deployment-dir`
- **AND** the help explains that `--deployment-dir` and `--deployment` cannot
  be used together

#### Scenario: Documentation explains legacy fallback
- **WHEN** a user reads deployment documentation
- **THEN** the documentation explains when named and implicit default
  selections use a deployment kept at the legacy location
- **AND** the documentation explains that the new location takes precedence
  when a matching path exists there

#### Scenario: Documentation warns about preserving deployment directories for cloud cleanup
- **WHEN** a user reads deployment documentation
- **THEN** the documentation explains that cloud deployment directories should
  be kept until cloud resources are destroyed

### Requirement: Launcher-managed deployment directory reuse SHALL avoid stale preset deployment
When commands resolve to the default deployment directory or to a named
deployment directory, they SHALL validate requested preset identity against
initialized local state before deploying resources.

#### Scenario: Same default directory install retry uses same preset state
- **WHEN** a user reruns `exasol install <infra-preset>` outside a recognized
  deployment directory
- **AND** the resolved default deployment directory is initialized with the
  same preset identity
- **THEN** the command reuses the default deployment directory
- **AND** the command does not remove local deployment files before deployment
  retry

#### Scenario: Different default directory preset is refused
- **WHEN** a user runs `exasol install <different-infra-preset>` outside a
  recognized deployment directory
- **AND** the resolved default deployment directory is initialized with
  another preset identity
- **THEN** the command fails before deployment
- **AND** the command does not deploy the stale preset from the default
  deployment directory
- **AND** the command tells the user to run `exasol destroy --remove` before
  initializing different presets, or `exasol remove` if the cloud resources
  are already gone

#### Scenario: Different named directory preset is refused
- **WHEN** a user runs
  `exasol install <different-infra-preset> --deployment <name>`
- **AND** the resolved `<name>` directory is initialized with another preset
  identity
- **THEN** the command fails before deployment
- **AND** the command does not deploy the stale preset from the named
  deployment directory
- **AND** the command tells the user to run `exasol destroy --remove` before
  initializing different presets, or `exasol remove` if the cloud resources
  are already gone
