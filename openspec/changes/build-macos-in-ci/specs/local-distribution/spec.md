## ADDED Requirements

### Requirement: macOS local launcher release builds use a macOS host

The release pipeline SHALL build the `darwin/arm64` local-mode launcher on a macOS CI runner rather than Linux cross-compilation.

#### Scenario: Release build targets the macOS local-mode launcher

- **GIVEN** a tagged release that includes the macOS local-mode launcher
- **WHEN** the release pipeline builds the `darwin/arm64` launcher artifact
- **THEN** that launcher build SHALL run on a macOS CI host
- **AND** the resulting launcher SHALL be the macOS artifact attached to the release

### Requirement: macOS launcher artifacts include the embedded runtime payload baseline

The release pipeline SHALL generate and embed the pinned local runtime payload baseline into the macOS launcher artifact during CI.

#### Scenario: Release build generates the embedded payload bundle

- **GIVEN** pinned release inputs for the local runtime `.run`, kernel, initrd, and payload version
- **WHEN** the macOS launcher build runs in CI
- **THEN** the pipeline SHALL generate the embedded payload bundle from those inputs
- **AND** the launcher artifact produced by that job SHALL include that embedded payload baseline

### Requirement: macOS launcher release artifacts are signed and notarized

The release pipeline SHALL ship the macOS local-mode launcher as a signed and notarized artifact with the required virtualization entitlement.

#### Scenario: Release build publishes the macOS launcher

- **GIVEN** a successful macOS launcher build in CI
- **WHEN** the pipeline prepares the macOS artifact for release publication
- **THEN** the launcher SHALL be signed with the virtualization entitlement
- **AND** the shipped macOS artifact SHALL be notarized before publication

### Requirement: macOS release failures block publication

The release pipeline SHALL fail the release when the macOS launcher build, embedded payload packaging, signing, or notarization steps fail.

#### Scenario: macOS release step fails

- **GIVEN** a tagged release that includes the macOS local-mode launcher
- **WHEN** any required macOS release step fails
- **THEN** the release pipeline SHALL report the release as failed
- **AND** the macOS launcher SHALL NOT be published as a successful release artifact
