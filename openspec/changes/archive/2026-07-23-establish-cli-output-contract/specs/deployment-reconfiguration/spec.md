# deployment-reconfiguration Specification

## MODIFIED Requirements

### Requirement: Init SHALL initialize preset extraction and patch same-preset supplied options

The `init` command SHALL initialize an empty deployment directory by extracting selected presets and writing launcher state. For an existing deployment with the same preset identity, `init` SHALL preserve local state and apply only supplied configuration options as a patch.

#### Scenario: Init initializes an empty deployment directory

- **WHEN** a user runs `exasol init <infra-preset> [install-preset]` for an empty deployment directory
- **THEN** the launcher extracts the selected presets into the deployment directory
- **AND** the launcher writes initialized deployment state with the selected preset identity
- **AND** the EULA notice is printed as a final terminal notice
- **AND** the EULA notice is not written to the deployment log file

#### Scenario: Init without options does not configure an existing same-preset deployment

- **WHEN** a user runs `exasol init <infra-preset> [install-preset]` for a deployment directory already initialized with the same preset identity
- **AND** the user did not supply configuration options
- **THEN** the command succeeds without removing local deployment files
- **AND** the command tells the user that configuration was not changed
- **AND** the command offers call-to-action guidance to use `exasol config set` for parameter changes

#### Scenario: Init patches supplied options for an existing same-preset deployment

- **WHEN** a user runs `exasol init <infra-preset> [install-preset] <configuration-options>` for a deployment directory already initialized with the same preset identity
- **THEN** the command succeeds without removing local deployment files
- **AND** the launcher updates only the supplied options in the corresponding local parameter files
- **AND** omitted options keep their current effective values
- **AND** the command prints the active effective configuration values on standard output
- **AND** the command offers call-to-action guidance to run `exasol deploy` to apply the changed configuration

#### Scenario: Init refuses a different preset

- **WHEN** a user runs `exasol init <different-infra-preset>` for an initialized deployment directory
- **THEN** the command fails without modifying local deployment files
- **AND** the error tells the user to run `exasol destroy --remove` before initializing different presets, or `exasol remove` if cloud resources are already gone

### Requirement: Config commands SHALL inspect, patch, and reset same-preset deployment parameters

The `config get`, `config set`, and `config reset` commands SHALL manage configuration parameter files for the initialized deployment's existing presets without deleting local deployment state, extracted presets, backend setup artifacts, OpenTofu state, credentials, or connection metadata.

#### Scenario: Config get prints active configuration

- **WHEN** a user runs `exasol config get` in an initialized deployment directory
- **THEN** the command prints the active effective configuration values on standard output
- **AND** the command supports `--json`

#### Scenario: Config get prints selected options

- **WHEN** a user runs `exasol config get <option-name> [<option-name>...]`
- **THEN** the command prints only the requested active configuration values on standard output
- **AND** the command fails when any requested option does not exist

#### Scenario: Config set patches parameters for initialized deployment

- **WHEN** a user runs `exasol config set <configuration-options>` in an initialized deployment directory
- **THEN** the launcher validates the options against the persisted preset manifests
- **AND** the command accepts preset-specific configuration options with the same `--option` flag style used by `init` and `install`
- **AND** the launcher updates only the supplied options in the corresponding local parameter files
- **AND** omitted options keep their current effective values
- **AND** the command prints the active effective configuration values on standard output
- **AND** the command offers call-to-action guidance to run `exasol deploy` to apply the changed configuration
- **AND** the apply guidance is not written to the deployment log file
- **AND** the launcher does not perform backend workspace setup
- **AND** the launcher preserves infrastructure state files

#### Scenario: Config reset restores selected defaults

- **WHEN** a user runs `exasol config reset <option-name> [<option-name>...]`
- **THEN** the launcher resets only the requested options to their preset defaults
- **AND** the command prints the active effective configuration values on standard output
- **AND** the command offers call-to-action guidance to run `exasol deploy` to apply the changed configuration

#### Scenario: Config reset all restores all defaults explicitly

- **WHEN** a user runs `exasol config reset --all`
- **THEN** the launcher resets all configurable options to their preset defaults
- **AND** the command prints the active effective configuration values on standard output
- **AND** the command offers call-to-action guidance to run `exasol deploy` to apply the changed configuration

#### Scenario: Config set and reset refuse any state with possibly-deployed cloud resources

- **WHEN** a deployment is in a state other than initialized (running, stopped, deployment-failed, interrupted during deploy or destroy, or with an operation in progress)
- **AND** a user runs `exasol config set <configuration-options>` or `exasol config reset <options>`
- **THEN** the command fails without updating configuration
- **AND** the error tells the user that the deployment may already have cloud resources
- **AND** the error tells the user to run `exasol destroy` (or `exasol remove` if the cloud resources are confirmed gone) before changing configuration and redeploying

#### Scenario: Config commands refuse uninitialized directories

- **WHEN** a user runs `exasol config get`, `exasol config set`, or `exasol config reset` for a deployment directory that is not initialized
- **THEN** the command fails with a message that the deployment directory must be initialized first
