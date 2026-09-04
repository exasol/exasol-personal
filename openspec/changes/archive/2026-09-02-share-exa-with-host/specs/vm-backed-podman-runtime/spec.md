## MODIFIED Requirements

### Requirement: macOS installs Nano through guest Podman

The system SHALL perform Nano image loading, SLC materialization, container
creation, status inspection, diagnostics, and cleanup through Podman commands
executed inside the VM while storing Nano's complete `/exa` tree in the managed
host share.

#### Scenario: Fresh macOS deployment starts

- **WHEN** the VM is running and the deployment has no running Nano container
- **THEN** the same persistent Podman installation behavior used by the Linux
  host runtime runs inside the VM with the managed host directory mounted at
  `/exa`

#### Scenario: macOS deployment stops

- **WHEN** a running macOS deployment is stopped
- **THEN** its disposable Nano container is removed before the VM is stopped
  and persistent host-side data remains

#### Scenario: macOS host path is prepared

- **WHEN** the launcher prepares a macOS local deployment
- **THEN** `local/runtime/exa` resolves to Nano data in the runner's private
  host share

#### Scenario: macOS host path conflicts

- **WHEN** the macOS `local/runtime/exa` path conflicts with the
  launcher-managed host path
- **THEN** preparation fails and preserves the existing path

### Requirement: legacy runner database assets are retired safely

The system SHALL remove obsolete host-shared runner database payloads, preserve
existing Nano data while migrating it from the macOS guest disk to managed host
storage, allow failed or interrupted migrations to be retried safely, retain a
recoverable guest backup until deployment destruction, and run a deployment on
the guest operating system of the runner that drives it.

#### Scenario: Existing deployment contains legacy shared payloads

- **WHEN** a deployment starts with VM-only runner support
- **THEN** obsolete shared Nano payload and runner configuration files are
  removed before installation

#### Scenario: Existing guest data is present

- **WHEN** a deployment created by an earlier runner has persistent database
  data in the VM data disk
- **THEN** the data remains available after the deployment starts from managed
  host storage and a recoverable guest backup remains available

#### Scenario: Data migration cannot complete

- **WHEN** migration fails or is interrupted before the deployment starts from
  managed host storage
- **THEN** the existing data remains unchanged and a later start can retry
  migration safely

#### Scenario: Guest and host data conflict

- **WHEN** both the legacy guest location and the managed host location contain
  different Nano data
- **THEN** startup fails, reports both locations, and preserves both data sets

#### Scenario: Migrated deployment starts again

- **WHEN** a deployment has started successfully from migrated host data
- **THEN** later starts reuse that host data without replacing it from the guest
  backup

#### Scenario: Deployment was created by a different runner

- **WHEN** a deployment starts and the runner recorded for it differs from the
  resolved runner, or no runner is recorded
- **THEN** the deployment runs the resolved runner's guest operating system and
  keeps its database and container state

#### Scenario: Deployment already matches the resolved runner

- **WHEN** a deployment starts and the runner recorded for it matches the
  resolved runner
- **THEN** the deployment keeps its guest operating system

#### Scenario: Guest replacement cannot complete

- **WHEN** replacing the guest operating system fails
- **THEN** startup fails and a later start replaces the guest again

#### Scenario: VM runtime state is present

- **WHEN** Nano data has moved to the managed host share
- **THEN** the runner continues using the VM data disk for Podman and other
  guest runtime state
