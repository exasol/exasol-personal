## ADDED Requirements

### Requirement: Local network reachability failures are classified distinctly

The system SHALL distinguish a local-runtime network reachability problem from a generic database-startup or connection failure when a local deployment's forwarded ports cannot be confirmed reachable.

#### Scenario: Reachability problem during install or start

- **WHEN** `exasol install local` or `exasol start` is waiting for the local database to become ready and the local runtime reports the forwarded ports as not reachable (rather than an authentication response), for longer than a bounded grace period
- **THEN** the launcher fails with a local runtime reachability error instead of a generic database-startup timeout or an indefinite wait

#### Scenario: Reachability problem during connect

- **WHEN** a local deployment is running and `exasol connect` cannot establish a connection because the local runtime reports the database port as not reachable
- **THEN** the launcher fails with a local runtime reachability error instead of a raw driver error

#### Scenario: Reachability problem during shell access

- **WHEN** a local deployment is running and `exasol shell host` or `exasol shell container` cannot establish an SSH connection because the local runtime reports the SSH port as not reachable
- **THEN** the launcher fails with a local runtime reachability error instead of a raw SSH connection error

#### Scenario: Generic failure remains generic

- **WHEN** a local database connection attempt fails and the local runtime reports the relevant port as reachable (the failure is not a network reachability problem)
- **THEN** the launcher reports the existing generic failure behavior for that command, unchanged

### Requirement: Reachability error explains the macOS Local Network permission cause

The system SHALL explain, in the reachability error message, that macOS's Local Network permission applies to the invoking terminal, editor, or agent environment even though the affected endpoint is a loopback address.

#### Scenario: Error names example invoking environments

- **WHEN** the launcher reports a local runtime reachability error
- **THEN** the message names example invoking environments (such as a terminal emulator, an editor, or an agent host) as the permission target, without asserting which specific application is actually responsible

#### Scenario: Error explains the loopback nuance

- **WHEN** the launcher reports a local runtime reachability error
- **THEN** the message explains that granting the permission is required even though the reported database endpoint is `127.0.0.1`, because the launcher forwards it from a VM

### Requirement: Reachability classification distinguishes network-wide from database-specific problems

The system SHALL check reachability of every forwarded local port, not only the port relevant to the current command, when classifying a failure.

#### Scenario: All forwarded ports unreachable

- **WHEN** the local runtime reports every forwarded port as not reachable
- **THEN** the launcher classifies the failure as a local runtime reachability problem

#### Scenario: Only the database port is affected

- **WHEN** the local runtime reports the SSH port as reachable but the database port as not reachable
- **THEN** the launcher does not classify the failure as a local runtime reachability problem, and instead reports the existing generic database failure behavior

### Requirement: Local diagnostics command

The system SHALL provide a read-only command that reports the local deployment's runtime and reachability state independent of any command failure.

#### Scenario: Diagnostics report runtime and reachability state

- **WHEN** a user runs `exasol diag local` against a local deployment
- **THEN** the launcher reports the local VM's running status, its reported guest IP, the bound host ports, the reachability of each forwarded port, and the database's SQL-level readiness

#### Scenario: Diagnostics report platform support

- **WHEN** a user runs `exasol diag local` on an unsupported operating system or architecture
- **THEN** the launcher reports that the local deployment preset is unsupported on the current platform, without requiring a running deployment

#### Scenario: Diagnostics on a supported platform with no VM running

- **WHEN** a user runs `exasol diag local` on a supported operating system and architecture and the local VM is not running
- **THEN** the launcher reports, concisely, that the platform is ready to run the local deployment, gives the simple instruction to start it, and notes that running `exasol diag local` again once it is running will report additional detail

#### Scenario: Diagnostics available without a failure

- **WHEN** a local deployment is healthy and reachable
- **THEN** `exasol diag local` reports success for each checked aspect rather than requiring a prior failure to run

#### Scenario: VM running inconsistently with the recorded deployment state

- **WHEN** a user runs `exasol diag local` and the local VM is running, but the recorded deployment state does not expect one to be (e.g. a process orphaned by a prior crash or a manually killed launcher invocation)
- **THEN** the launcher reports this inconsistency as an explicit warning, alongside whatever other diagnostics it can still gather, explaining that this can cause a future `start`/`install` to fail with a VM storage conflict and suggesting the user look for and stop the stray process before retrying
