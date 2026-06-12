## ADDED Requirements

### Requirement: Optimized launcher binary builds
The build system SHALL apply the approved low-risk Go binary size optimizations to launcher binaries for supported release targets without removing user-visible CLI behavior.

#### Scenario: Building launcher binaries
- **WHEN** a launcher binary is built through the project build or release workflow
- **THEN** the build uses the approved Go-native size optimization flags
- **AND** the resulting binary preserves the launcher command behavior

### Requirement: Higher-risk optimizations remain excluded
The project SHALL NOT use executable packing, alternative Go compiler toolchains, or dependency replacement solely for raw binary size reduction without a separate design decision.

#### Scenario: Evaluating binary size techniques
- **WHEN** a higher-risk binary size technique is considered
- **THEN** the project documents the release, signing, debugging, and maintenance tradeoffs before adopting it
