## MODIFIED Requirements

### Requirement: Install SHALL orchestrate initialization, configuration, and deployment
The `install` command SHALL combine initialization, configuration, and deployment in a state-aware way that preserves retry safety. Its EULA notice SHALL be emitted only after the full install succeeds.

#### Scenario: Install initializes and deploys a new deployment
- **WHEN** a user runs `exasol install <infra-preset> [install-preset] <configuration-options>` for an empty deployment directory
- **THEN** the launcher initializes the deployment directory with the selected presets
- **AND** the launcher applies the supplied configuration options
- **AND** the launcher deploys the initialized deployment
- **AND** final connection instructions are printed after deployment log cleanup
- **AND** the EULA is the final operational notice, emitted after deployment succeeds

#### Scenario: Install configures and retries same-preset deployment
- **WHEN** a user reruns `exasol install <infra-preset> [install-preset] <configuration-options>` for a deployment directory initialized with the same preset identity
- **THEN** the launcher applies the supplied configuration options as a patch without removing local deployment files
- **AND** omitted options keep their current effective values
- **AND** the launcher runs deployment using the preserved local infrastructure state
- **AND** final connection instructions are printed after deployment log cleanup
- **AND** the EULA is the final operational notice, emitted after deployment succeeds

#### Scenario: Install refuses different preset
- **WHEN** a user runs `exasol install <different-infra-preset>` for an initialized deployment directory
- **THEN** the command fails before deployment
- **AND** the command does not remove local deployment files
- **AND** the message tells the user to run `exasol destroy --remove` before initializing different presets, or `exasol remove` if cloud resources are already gone

#### Scenario: Failed deployment has no EULA notice
- **WHEN** initialization succeeds but the deployment phase of `exasol install` fails
- **THEN** the command reports the deployment failure
- **AND** the EULA notice is not emitted
