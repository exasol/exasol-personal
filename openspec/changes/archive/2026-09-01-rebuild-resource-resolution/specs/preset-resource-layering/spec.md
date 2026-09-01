## ADDED Requirements

### Requirement: A preset SHALL declare its own resource specification
An infrastructure or installation preset SHALL be able to ship a resource
specification of its own, using the same format the launcher uses. Resources it
declares SHALL be resolvable while that preset is being evaluated.

#### Scenario: Preset-declared resource is resolvable
- **WHEN** a preset ships a resource specification declaring a resource
- **AND** that preset is being evaluated
- **THEN** the launcher resolves that resource

#### Scenario: Preset without a specification is unaffected
- **WHEN** a preset ships no resource specification
- **THEN** the launcher resolves resources exactly as it does for any other
  preset

#### Scenario: Invalid preset specification is reported
- **WHEN** a preset ships a resource specification that is not valid
- **THEN** the launcher reports an error naming the preset

### Requirement: Preset selections SHALL resolve from launcher resources
The launcher SHALL resolve the selected infrastructure and installation presets
before applying either preset's resource layer.

#### Scenario: Launcher-selected installation preset remains selected
- **WHEN** an infrastructure preset declares a resource matching the selected
  installation preset
- **THEN** the launcher evaluates the installation preset selected from launcher
  resources

#### Scenario: Launcher-selected infrastructure preset remains selected
- **WHEN** an installation preset declares a resource matching the selected
  infrastructure preset
- **THEN** the launcher evaluates the infrastructure preset selected from
  launcher resources

### Requirement: Preset resources SHALL be scoped to one evaluation
Resources declared by a preset SHALL be available only while that preset is
being evaluated. Each preset's layer SHALL derive directly from the launcher
resources.

#### Scenario: Preset resource is unavailable after evaluation
- **WHEN** a preset declaring a resource has finished being evaluated
- **THEN** that resource is no longer resolvable by name

#### Scenario: One preset's resources do not reach another
- **WHEN** two presets each declare a resource under the same name
- **THEN** each preset resolves its own declaration while it is being evaluated

### Requirement: Preset resources SHALL override launcher resources
A resource declared by a preset SHALL take precedence over a launcher resource
of the same name for the duration of that preset's evaluation, and the
launcher's own declaration SHALL apply again afterwards.

#### Scenario: Preset overrides a launcher resource
- **WHEN** a preset declares a resource under a name the launcher also declares
- **AND** that preset is being evaluated
- **THEN** the launcher resolves the preset's declaration

#### Scenario: Launcher declaration applies again after evaluation
- **WHEN** evaluation of a preset that overrode a launcher resource has finished
- **THEN** the launcher resolves its own declaration for that name again

#### Scenario: Overriding declarations do not share a cached artifact
- **WHEN** a preset overrides a launcher resource with different content
- **THEN** each declaration resolves to its own cached artifact

### Requirement: Relative preset locations SHALL resolve against the preset
A relative local location declared in a preset's resource specification SHALL
resolve against that preset's own directory, so a preset can address content it
ships.

#### Scenario: Preset addresses its own content
- **WHEN** a preset's resource specification declares a relative local location
- **THEN** the launcher resolves it against the preset's own directory

#### Scenario: Working directory does not affect resolution
- **WHEN** a preset's resource specification declares a relative local location
- **AND** the launcher is invoked from different working directories
- **THEN** the location resolves to the same content each time

### Requirement: Preset build directives SHALL be ignored with a warning
A preset's resource specification SHALL NOT be able to request embedding or
build-time expansion, since neither can apply after the launcher binary is
built. The launcher SHALL ignore such a directive and warn, rather than
failing.

#### Scenario: Embedding directive is ignored
- **WHEN** a preset's resource specification declares an embedding directive
- **THEN** the launcher ignores it and emits a warning naming the directive and
  the preset
- **AND** the resource resolves from its declared source

#### Scenario: Expansion pattern is ignored
- **WHEN** a preset's resource specification declares an expansion pattern
- **THEN** the launcher ignores it and emits a warning naming the directive and
  the preset
