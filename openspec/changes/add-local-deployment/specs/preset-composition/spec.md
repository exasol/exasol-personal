# preset-composition Specification

## ADDED Requirements

### Requirement: Infrastructure and installation preset compatibility

The system SHALL validate that the selected installation preset is compatible with the selected infrastructure preset before initialization proceeds.

#### Scenario: Valid cloud preset pair

- GIVEN the infrastructure preset is `aws`
- AND the installation preset is `ubuntu`
- WHEN the user runs `exasol init aws ubuntu`
- THEN the launcher accepts the preset pair

#### Scenario: Invalid cloud and local pair

- GIVEN the infrastructure preset is `aws`
- AND the installation preset is `nano`
- WHEN the user runs `exasol init aws nano`
- THEN the launcher rejects the preset pair before mutating deployment state

#### Scenario: Invalid local and cloud pair

- GIVEN the infrastructure preset is `local`
- AND the installation preset is `ubuntu`
- WHEN the user runs `exasol init local ubuntu`
- THEN the launcher rejects the preset pair before mutating deployment state

### Requirement: Directional compatibility metadata

The system SHALL model preset compatibility through infrastructure-provided capabilities and installation-required capabilities.

#### Scenario: Infrastructure provides compatibility tags

- GIVEN an infrastructure preset defines compatibility metadata
- WHEN the launcher loads the infrastructure manifest
- THEN it reads the capabilities the infrastructure preset provides

#### Scenario: Installation requires compatibility tags

- GIVEN an installation preset defines compatibility metadata
- WHEN the launcher loads the installation manifest
- THEN it reads the capabilities the installation preset requires

#### Scenario: Compatibility validation uses required capabilities

- GIVEN an installation preset requires capabilities
- WHEN the launcher validates the selected preset pair
- THEN the infrastructure preset must provide every required capability
- AND the launcher reports any missing capabilities in the validation error

#### Scenario: Backend environment validation is generic

- GIVEN an infrastructure preset resolves to a backend
- WHEN the launcher validates that preset before mutating deployment state
- THEN the launcher invokes generic backend environment validation
- AND the validation hook is not limited to host-platform checks

### Requirement: Local default installation resolution

The system SHALL resolve omitted installation presets only to installation presets that are compatible with the selected infrastructure preset.

#### Scenario: Local default resolves to compatible local installation

- GIVEN the user runs `exasol install local`
- WHEN the launcher chooses a default installation preset
- THEN it resolves to a local-compatible installation preset such as `nano`

#### Scenario: Omitted installation never bypasses compatibility rules

- GIVEN the user omits the installation preset
- WHEN the launcher resolves the default installation preset
- THEN the resolved preset must still pass compatibility validation against the selected infrastructure preset

### Requirement: Compatibility discoverability

The system SHALL make embedded preset compatibility obvious in CLI help and preset discovery output, and SHALL call out when a preset is special rather than a regular cloud preset.

#### Scenario: Init help highlights the special local preset and compatibility matrix

- GIVEN the launcher includes embedded infrastructure and installation presets
- WHEN the user runs `exasol init --help`
- THEN the help output explicitly calls out `local` as a special built-in preset
- AND the help output shows the embedded preset compatibility matrix

#### Scenario: Preset listing surfaces compatibility relationships

- GIVEN the launcher includes embedded infrastructure and installation presets
- WHEN the user runs `exasol presets list`
- THEN the output shows the embedded preset compatibility matrix
- AND the output calls out `local` as a special built-in preset
