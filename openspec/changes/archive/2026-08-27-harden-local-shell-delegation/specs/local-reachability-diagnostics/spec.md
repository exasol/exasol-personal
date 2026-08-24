## ADDED Requirements

### Requirement: Local command failures are actionable and accurate

The CLI SHALL identify recognized connectivity problems affecting a local deployment, provide platform-appropriate corrective guidance, include the operation's actual failure, and treat command-specific failures as authoritative when connectivity is not established as a relevant cause.

#### Scenario: Blocked local access receives corrective guidance

- **WHEN** a local-deployment command fails because access to the deployment appears to be blocked by the host environment
- **THEN** the command reports the connectivity problem and provides platform-appropriate steps for restoring access

#### Scenario: Connectivity guidance retains the operation failure

- **WHEN** the CLI adds connectivity guidance to a failed local-deployment operation
- **THEN** the error also reports the underlying operation failure

#### Scenario: Command-specific failure remains authoritative

- **WHEN** a local-deployment command fails and blocked access cannot be established as a relevant cause
- **THEN** the CLI reports the command-specific failure as the authoritative error

### Requirement: Local diagnostics report deployment usability

The CLI SHALL provide a read-only `exasol diag local` command that summarizes whether the current platform and local deployment are ready for use independently of any prior command failure.

#### Scenario: Diagnostics report deployment usability

- **WHEN** a user runs `exasol diag local` against a running local deployment
- **THEN** the command reports whether the deployment is running, whether its exposed services are reachable, and whether the database is ready

#### Scenario: Diagnostics report platform support

- **WHEN** a user runs `exasol diag local` on an unsupported operating system or architecture
- **THEN** the command reports from the current platform state that the local deployment preset is unsupported

#### Scenario: Diagnostics run without an active deployment

- **WHEN** a user runs `exasol diag local` on a supported platform without an active local deployment
- **THEN** the command reports that the platform is ready, explains how to start the deployment, and notes that rerunning diagnostics afterward will provide additional detail

#### Scenario: Diagnostics report inconsistent deployment state

- **WHEN** local deployment resources remain active even though the recorded deployment state does not expect them
- **THEN** `exasol diag local` reports an explicit warning and corrective guidance before the user retries a lifecycle command

## REMOVED Requirements

### Requirement: Local diagnostics command

**Reason**: The requirement encodes runtime, VM, transport, and individual port details instead of the durable diagnostic outcome visible to users.

**Migration**: Use `Local diagnostics report deployment usability`, which summarizes platform support, deployment state, exposed-service reachability, database readiness, and corrective guidance through `exasol diag local`.

### Requirement: Local network reachability failures are classified distinctly

**Reason**: The requirement enumerates commands and transport observations instead of specifying the durable user-facing error behavior.

**Migration**: Use `Local command failures are actionable and accurate`, which requires recognized connectivity failures to provide corrective guidance without masking the operation failure.

### Requirement: Reachability error explains the macOS Local Network permission cause

**Reason**: Platform-specific guidance remains required, but its permanent requirement does not need to encode the current forwarding architecture or fixed endpoint details.

**Migration**: Use `Local command failures are actionable and accurate`; macOS guidance continues to direct users to grant Local Network access to the invoking application when that is the recognized cause.

### Requirement: Reachability classification distinguishes network-wide from database-specific problems

**Reason**: Endpoint comparison and classification states are internal diagnostic policy rather than behavior consumed through the CLI.

**Migration**: Use `Local command failures are actionable and accurate`, which preserves unrelated failures whenever blocked access is not established as a relevant cause.
