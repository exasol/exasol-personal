# preset-contextual-help Specification

## Purpose
Defines how the help of `exasol install` and `exasol init` adapts to the presets selected on the command line: what a user reads about a preset they selected, and what they read when they have not selected one.

## Requirements
### Requirement: Help for a selected infrastructure preset identifies and describes it
`exasol install` and `exasol init` SHALL identify the infrastructure preset selected by their first preset argument, describe it, and show the parameters it supports.

#### Scenario: Install help describes the selected infrastructure preset
- **WHEN** a user runs `exasol install <infra-preset> --help`
- **THEN** the help identifies the selected infrastructure preset by the name given on the command line
- **AND** the help shows that preset's description
- **AND** the help shows the parameters that preset supports

#### Scenario: Init help describes the selected infrastructure preset
- **WHEN** a user runs `exasol init <infra-preset> --help`
- **THEN** the help identifies the selected infrastructure preset by the name given on the command line
- **AND** the help shows that preset's description

#### Scenario: A preset selected by directory path is identified by that path
- **WHEN** a user requests help with a preset directory path as the preset argument
- **THEN** the help identifies the selected preset by the path the user wrote
- **AND** the help shows the description recorded in that preset directory

### Requirement: Help for a selected infrastructure preset names its compatible installation presets
`exasol install` and `exasol init` SHALL name the installation presets that can be deployed on the selected infrastructure preset.

#### Scenario: Compatible installation presets are named
- **WHEN** a user requests help with an infrastructure preset that has compatible installation presets
- **THEN** the help names each compatible installation preset

#### Scenario: An infrastructure preset without compatible installation presets reports none
- **WHEN** a user requests help with an infrastructure preset that has no compatible installation preset
- **THEN** the help reports that no installation preset is compatible

### Requirement: Help for a selected installation preset identifies and describes it
`exasol install` and `exasol init` SHALL identify the installation preset selected by their second preset argument and describe it.

#### Scenario: Both selected presets are described
- **WHEN** a user runs `exasol install <infra-preset> <install-preset> --help`
- **THEN** the help identifies the selected installation preset by the name given on the command line
- **AND** the help shows that preset's description
- **AND** the help describes the selected infrastructure preset

### Requirement: Help with a preset selected describes only the selected presets
When a preset argument selects a preset, `exasol install` and `exasol init` SHALL limit the presets their help describes to the selected ones.

#### Scenario: One selected preset is the only preset described
- **WHEN** a user runs `exasol install <infra-preset> --help`
- **THEN** the only infrastructure preset the help describes is the selected one

#### Scenario: Two selected presets are the only presets described
- **WHEN** a user runs `exasol install <infra-preset> <install-preset> --help`
- **THEN** the only presets the help describes are the two selected ones

### Requirement: Help shows the usage and examples for the selected presets
`exasol install` and `exasol init` SHALL show a usage line and examples that invoke the command with the presets selected on the command line.

#### Scenario: Usage and examples use a selected infrastructure preset
- **WHEN** a user requests help with an infrastructure preset selected
- **THEN** the usage line names the selected preset in the position of the infrastructure preset argument
- **AND** an example invokes the command with the selected preset

#### Scenario: An example pairs the selected preset with a compatible installation preset
- **WHEN** a user requests help with an infrastructure preset that has compatible installation presets
- **THEN** an example invokes the command with the selected preset and one of those compatible installation presets

#### Scenario: Usage and examples repeat the preset argument as the user wrote it
- **WHEN** a user requests help with a preset selected by a path
- **THEN** the usage line and the examples name that preset by the argument the user wrote
- **AND** an argument containing spaces is quoted so that it stays one argument when an example is run

#### Scenario: Examples use both selected presets
- **WHEN** a user requests help with an infrastructure preset and an installation preset selected
- **THEN** the usage line names both selected presets
- **AND** an example invokes the command with both selected presets

### Requirement: Help without a selected preset lists every built-in preset and how they combine
`exasol install` and `exasol init` SHALL list the built-in infrastructure and installation presets and show which of them are compatible whenever no preset argument selects a preset.

#### Scenario: Help without a preset argument lists every built-in preset
- **WHEN** a user runs `exasol install --help` or `exasol init --help`
- **THEN** the help lists the built-in infrastructure presets
- **AND** the help lists the built-in installation presets
- **AND** the help shows which infrastructure and installation presets are compatible with each other

#### Scenario: Help with a preset argument that selects no preset lists every built-in preset
- **WHEN** a user requests help with a preset argument that names neither a built-in preset nor a readable preset directory
- **THEN** the command prints help and succeeds
- **AND** the help lists the built-in presets and shows which of them are compatible with each other

### Requirement: Command help points to preset discovery
`exasol install` and `exasol init` SHALL point users to the command that lists and exports presets.

#### Scenario: Help names the preset discovery command
- **WHEN** a user requests help with or without a preset selected
- **THEN** the help names the command that lists and exports presets
