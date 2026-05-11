# local-distribution Specification

## ADDED Requirements

### Requirement: macOS arm64 launcher distribution for local mode

The system SHALL support local mode through the macOS arm64 launcher distribution without requiring a separate native ExaNano wrapper binary.

#### Scenario: Launcher contains host-side local runtime control

- GIVEN the user installs the macOS arm64 launcher distribution
- WHEN the user runs local deployment commands
- THEN the launcher provides the host-side virtualization and orchestration layer itself

#### Scenario: Launcher contains embedded local runtime payload artifacts

- GIVEN the user installs the macOS arm64 launcher distribution
- WHEN the launcher is built for local mode
- THEN the launcher also contains the compressed embedded local runtime payload bundle required for the local VM guest baseline

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

### Requirement: Local-mode release packaging support

The system SHALL package the embedded local runtime payload bundle and the launcher in a coordinated build and release flow.

#### Scenario: Build local-mode launcher with embedded guest payload

- GIVEN a release includes local deployment support
- WHEN the macOS arm64 launcher is built
- THEN the Linux ExaNano `.run` payload and required boot assets are compressed and embedded into that launcher build

#### Scenario: Installer-ready launcher release

- GIVEN a supported macOS arm64 user installs the launcher
- WHEN they run `exasol install local`
- THEN the launcher can resolve the required local payload artifacts from the embedded bundle and the local cache without a remote payload fetch
