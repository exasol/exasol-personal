# local-deployment Specification (Delta)

## MODIFIED Requirements

### Requirement: Local deployment mode

The system SHALL provide a built-in `local` infrastructure preset for Apple
Silicon macOS hosts running macOS 13 (Ventura) or later.

#### Scenario: Initialize local deployment on a supported host

- GIVEN the user is running the launcher on Apple Silicon macOS 13 or later
- WHEN the user runs `exasol init local`
- THEN the launcher initializes a deployment directory for local deployment
- AND the launcher does not require cloud credentials

#### Scenario: Reject unsupported host platform

- GIVEN the user is running on a host that is not Apple Silicon macOS, or on a
  macOS version earlier than 13 (Ventura)
- WHEN the user runs `exasol init local`
- THEN the launcher fails before mutating deployment state
- AND the error explains that local deployment requires Apple Silicon macOS 13
  or later because EFI VM boot is unavailable on earlier versions

### Requirement: Dedicated local lifecycle backend

The system SHALL execute local deployment lifecycle operations through a
dedicated local backend instead of OpenTofu, SSH-oriented node operations, or
cloud power-state helpers.

#### Scenario: Deploy local runtime

- GIVEN an initialized local deployment directory
- WHEN the user runs `exasol deploy`
- THEN the launcher boots its own local VM from the staged EFI disk image
  through the local backend
- AND the launcher waits until the database accepts a probe connection on the
  forwarded loopback SQL port before reporting the deployment as running

#### Scenario: Restart local deployment

- GIVEN a stopped local deployment
- WHEN the user runs `exasol start`
- THEN the launcher restarts the deployment through the local backend
- AND the launcher reuses the deployment's persisted local runtime
  configuration, staged disk image, EFI variable store, and staged payload
  share contents
- AND the launcher rewrites the staged start script from its embedded copy
  before booting the VM

#### Scenario: Destroy local deployment

- GIVEN a local deployment exists
- WHEN the user runs `exasol destroy`
- THEN the launcher stops the local runtime if needed
- AND removes deployment-owned local runtime artifacts including the staged
  disk image, EFI variable store, and the staged payload share directory
- AND returns the deployment to the initialized state

## ADDED Requirements

### Requirement: EFI disk boot

The system SHALL boot the local VM from the staged EFI disk image using Apple
Virtualization.framework's EFI bootloader.

#### Scenario: Boot uses EFI bootloader

- GIVEN a local deployment with a staged EFI disk image and EFI variable store
- WHEN the launcher starts the VM
- THEN it configures the VM with an EFI bootloader pointing at the
  per-deployment EFI variable store
- AND it does not configure a Linux kernel, initrd, or kernel command line

#### Scenario: EFI variables persist across reboots

- GIVEN a local deployment that has been started at least once
- WHEN the launcher restarts the VM
- THEN it reuses the existing per-deployment EFI variable store file rather
  than creating a fresh one

### Requirement: Database start via guest bootstrap service

The system SHALL rely on the guest image's `exasol-bootstrap` OpenRC service to
execute the launcher-staged start script (`/mnt/host/start.sh`) on every boot.
The launcher itself does not connect into the guest to start the database.

#### Scenario: Database starts on first boot

- GIVEN a local deployment whose payload share has been staged with the
  installer binary and a launcher-authored start script
- WHEN the VM boots for the first time
- THEN the guest's `exasol-bootstrap` OpenRC service runs `start.sh` from
  `/mnt/host`
- AND `start.sh` invokes the installer binary so that the database starts and
  binds the SQL port

#### Scenario: Database starts again on subsequent boots

- GIVEN a local deployment that has been stopped and is being started again
- WHEN the VM boots
- THEN the guest's `exasol-bootstrap` OpenRC service runs the same staged
  `start.sh` again, restarting the database

### Requirement: Database-readiness probe via forwarded loopback

The system SHALL determine local deployment readiness by probing the database
connection through the host-side loopback forwarder rather than by reading a
guest-written runtime state file.

#### Scenario: Deploy waits for the database to accept connections

- GIVEN the launcher has started the local VM and the loopback forwarders
- WHEN the launcher waits for the deployment to be ready
- THEN it polls a database connection through the forwarded loopback port
- AND it reports the deployment as running once the probe succeeds within the
  configured timeout

#### Scenario: Deploy fails when the runner exits before readiness

- GIVEN the launcher is waiting for the local deployment to become ready
- WHEN the runner process exits before the database accepts a connection
- THEN the launcher fails with a clear error referencing the runner log file

### Requirement: Stop via VZ ACPI shutdown

The system SHALL stop the local VM via Apple Virtualization.framework's ACPI
shutdown request triggered by a host-side stop-request file, rather than by
sending a graceful-stop command to a guest-side control socket.

#### Scenario: Stop request triggers ACPI shutdown

- GIVEN a running local deployment booted from the EFI disk image
- WHEN the user runs `exasol stop`
- THEN the launcher writes a stop-request file in the deployment-owned control
  directory
- AND the runner observes the file and requests an ACPI shutdown of the VM
- AND the VM exits cleanly within the configured stop timeout
- AND the runner process exits and the launcher reports the deployment as
  stopped

#### Scenario: Stop falls back to a forced kill on timeout

- GIVEN a stop request has been issued but the VM does not exit within the
  configured stop timeout
- WHEN the launcher proceeds to enforce shutdown
- THEN it kills the runner process directly
- AND it reports the deployment as stopped after the runner has exited

## REMOVED Requirements

### Requirement: Guest-driven runtime state advertisement

This requirement asserted that the guest writes a runtime-state file consumed
by the host. The EFI-booted Alpine guest does not implement this contract;
readiness and shutdown are determined by host-observable behavior (port probe
and ACPI shutdown). The control-channel abstraction is preserved in the code
base for potential future use.
