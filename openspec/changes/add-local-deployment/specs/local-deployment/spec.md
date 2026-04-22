# local-deployment Specification

## ADDED Requirements

### Requirement: Local deployment mode

The system SHALL provide a built-in `local` infrastructure preset for Apple Silicon macOS hosts.

#### Scenario: Initialize local deployment on a supported host

- GIVEN the user is running the launcher on Apple Silicon macOS
- WHEN the user runs `exasol init local`
- THEN the launcher initializes a deployment directory for local deployment
- AND the launcher does not require cloud credentials

#### Scenario: Reject unsupported host platform

- GIVEN the user is not running on Apple Silicon macOS
- WHEN the user runs `exasol init local`
- THEN the launcher fails before mutating deployment state
- AND the error explains that local deployment is unsupported on that host platform

### Requirement: Dedicated local lifecycle backend

The system SHALL execute local deployment lifecycle operations through a dedicated local backend instead of OpenTofu, SSH-oriented node operations, or cloud power-state helpers.

#### Scenario: Deploy local runtime

- GIVEN an initialized local deployment directory
- WHEN the user runs `exasol deploy`
- THEN the launcher starts the local runtime through the local backend
- AND the launcher boots its own local VM and invokes the selected Linux ExaNano `.run` payload inside that guest
- AND the launcher waits for the database to become ready
- AND the launcher marks the deployment as running

#### Scenario: Restart local deployment

- GIVEN a stopped local deployment
- WHEN the user runs `exasol start`
- THEN the launcher restarts the deployment through the local backend
- AND the launcher reuses the deployment's persisted local runtime configuration

#### Scenario: Destroy local deployment

- GIVEN a local deployment exists
- WHEN the user runs `exasol destroy`
- THEN the launcher stops the local runtime if needed
- AND removes deployment-owned local runtime artifacts
- AND returns the deployment to the initialized state

### Requirement: Backend-owned deployment interactions

The system SHALL resolve the deployment backend before executing backend-specific deployment-directory behavior, and SHALL keep backend-private artifacts and schemas behind backend-owned interfaces instead of command-layer branching on backend names or file layouts.

#### Scenario: Diagnostic info is delegated to the backend

- GIVEN a deployment directory has already been initialized
- WHEN the user runs `exasol diag info`
- THEN the launcher resolves the deployment backend first
- AND the launcher reads the backend-produced deployment info through a common deployment-info contract
- AND the command layer does not read backend-private deployment artifacts directly

#### Scenario: Shell behavior is delegated to the backend

- GIVEN a deployment directory has already been initialized
- WHEN the user runs `exasol shell host`
- THEN the launcher resolves the deployment backend first
- AND the backend decides whether host-shell access exists
- AND unsupported local-shell behavior is produced by the local backend instead of command-layer special casing

#### Scenario: Backends return data while the launcher formats it

- GIVEN a deployment-directory command needs deployment metadata
- WHEN the launcher renders text or JSON output
- THEN backend-specific code provides data and operations
- AND common launcher code owns JSON encoding and text formatting

### Requirement: Deployment-scoped local runtime state

The system SHALL keep local runtime state isolated per deployment directory.

#### Scenario: Deployment owns local runtime root

- GIVEN a local deployment has been initialized
- WHEN the launcher prepares local runtime state
- THEN it stores local runtime artifacts under the deployment directory
- AND it does not use a shared default runtime directory for deployment-owned state

#### Scenario: Concurrent local deployments remain isolated

- GIVEN two different deployment directories contain local deployments
- WHEN both deployments are initialized or running
- THEN their runtime roots, logs, control paths, and persistent data remain isolated from each other

### Requirement: Local command behavior

The system SHALL provide explicit command behavior for local deployments, including clear unsupported-command errors when no honest local equivalent exists.

#### Scenario: Unsupported shell command on local deployment

- GIVEN a local deployment exists
- WHEN the user runs `exasol shell host`
- THEN the launcher fails with a clear unsupported message
- AND the message does not imply that SSH access exists

#### Scenario: Local diagnostic info

- GIVEN a local deployment exists
- WHEN the user runs `exasol diag info`
- THEN the launcher returns useful local diagnostic metadata
- AND includes local runtime paths or state relevant to debugging

### Requirement: Local-safe deployment artifacts

The system SHALL generate deployment artifacts for local deployments that support `info`, `connect`, and status reporting without requiring cloud-specific fields.

#### Scenario: Connect to local deployment

- GIVEN a local deployment is running
- WHEN the user runs `exasol connect`
- THEN the launcher uses deployment-owned connection information for the local runtime
- AND it does not require SSH metadata to connect

#### Scenario: Local connection instructions

- GIVEN a local deployment is running
- WHEN the user runs `exasol info`
- THEN the launcher renders connection details appropriate for local loopback access
- AND includes the deployment's local database and UI endpoints

### Requirement: Common deployment info schema

The system SHALL use a single launcher-facing deployment-info schema for deployment-directory interaction, with backend-specific sections represented through optional fields instead of separate file shapes per backend.

#### Scenario: Local and cloud populate the same deployment info contract

- GIVEN two deployment directories use different backends
- WHEN each backend writes `deployment.json`
- THEN both files follow the same launcher-facing schema
- AND fields without meaningful values may be omitted or null
