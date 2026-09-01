## MODIFIED Requirements

### Requirement: Plain preset name resolves to an embedded preset
A preset argument SHALL be matched against the binary's embedded preset catalog before being treated as a location. A plain identifier (no URI scheme, no filesystem path separators) naming an embedded preset SHALL resolve to it and SHALL NOT trigger any external fetch, even when a directory of the same name exists in the working directory.

#### Scenario: Known embedded name uses embedded asset
- **WHEN** the user passes a plain name that matches an embedded preset
- **THEN** the system SHALL use the embedded asset

#### Scenario: Embedded name is not shadowed by a local directory
- **WHEN** the user passes a plain name that matches an embedded preset
- **AND** a directory of the same name exists in the working directory
- **THEN** the system SHALL use the embedded asset

#### Scenario: Unknown plain name returns a descriptive error
- **WHEN** the user passes a plain name that does not match any embedded preset
- **THEN** the system SHALL return an error that includes the unknown name and the list of available embedded preset names

#### Scenario: Unreachable location reports a fetch failure
- **WHEN** the user passes an argument that names no embedded preset
- **AND** the argument carries a URI scheme or resembles a filesystem path
- **THEN** the system SHALL return an error describing why the location could not
  be fetched, rather than listing available preset names
