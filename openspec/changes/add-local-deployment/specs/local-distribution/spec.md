# local-distribution Specification

## ADDED Requirements

### Requirement: macOS arm64 launcher distribution for local mode

The system SHALL support local mode through the macOS arm64 launcher distribution without requiring a separate native ExaNano wrapper binary.

#### Scenario: Launcher contains host-side local runtime control

- GIVEN the user installs the macOS arm64 launcher distribution
- WHEN the user runs local deployment commands
- THEN the launcher provides the host-side virtualization and orchestration layer itself

### Requirement: Platform-isolated virtualization build

The system SHALL isolate local virtualization support to the macOS arm64 launcher build.

#### Scenario: Unsupported platform build path

- GIVEN the launcher is built for a non-macOS or non-arm64 target
- WHEN that build includes local deployment code paths
- THEN unsupported platform stubs are used instead of macOS virtualization bindings

#### Scenario: macOS arm64 build path

- GIVEN the launcher is built for macOS arm64
- WHEN local deployment support is included
- THEN the build uses the platform bridge required for the virtualization layer

### Requirement: Local-mode release support

The system SHALL publish the local runtime payload artifacts and the launcher in a coordinated release flow.

#### Scenario: Publish payload artifacts for launcher use

- GIVEN a release includes local deployment support
- WHEN release artifacts are published
- THEN the versioned Linux ExaNano `.run` payloads needed by local mode are published to the product-owned HTTP location
- AND any supporting guest boot assets required to start the launcher-owned VM are published alongside them

#### Scenario: Installer-ready launcher release

- GIVEN a supported macOS arm64 user installs the launcher
- WHEN they run `exasol install local`
- THEN the launcher can resolve the required local payload artifacts through the configured distribution flow
