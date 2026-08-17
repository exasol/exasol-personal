## ADDED Requirements

### Requirement: Standard builds follow the binary size policy
The project SHALL keep launcher distribution binaries size-conscious by applying a documented binary size policy in standard build and release workflows without removing user-visible CLI behavior.

#### Scenario: Producing launcher binaries
- **WHEN** a launcher binary is produced through the standard project build or release workflow
- **THEN** the build follows the documented binary size policy
- **AND** the resulting binary preserves supported launcher command behavior for the target platform

### Requirement: Developer builds can preserve debugger support
The project SHALL provide a documented development build mode for launcher binaries that preserves debugger support when size optimization would make troubleshooting harder.

#### Scenario: Building a launcher binary for debugging
- **WHEN** a developer requests a launcher binary for debugging through the project build workflow
- **THEN** the build preserves debugger-relevant information and avoids size-focused transformations that would make local troubleshooting harder
- **AND** the resulting binary preserves supported launcher command behavior for the target platform

### Requirement: Size optimizations preserve operational supportability
The project SHALL adopt binary size optimizations only when their release, signing, troubleshooting, and maintenance tradeoffs are acceptable for the default distribution.

#### Scenario: Evaluating a new size optimization technique
- **WHEN** a binary size optimization technique would change release, signing, troubleshooting, or maintenance characteristics
- **THEN** the project documents the tradeoffs before adopting it as part of the default binary size policy
