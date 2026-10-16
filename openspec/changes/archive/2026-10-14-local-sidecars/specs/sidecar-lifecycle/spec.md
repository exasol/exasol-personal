## Purpose

Let users manage enabled sidecars with their deployment and reconcile saved
configuration while keeping service failures and cleanup understandable.

## ADDED Requirements

### Requirement: Sidecars require a supporting deployment backend

Sidecar commands SHALL run on deployments whose backend runs sidecars. A
deployment whose backend does not SHALL report that sidecars are unsupported
for its deployment type, leaving its saved configuration unchanged.

#### Scenario: Unsupported deployment reports the limitation

- **WHEN** a user runs a sidecar command on a deployment whose backend does not
  run sidecars
- **THEN** the command reports that sidecars are unsupported for that
  deployment type and changes no saved configuration

### Requirement: Enablement follows deployment running state

Enabling a sidecar SHALL persist its definition and immediately ensure its
container is running on the deployment host when the deployment is running.
Local non-bridge deployments follow the explicit restart migration contract in
sidecar-networking. For initialized or stopped deployments, enablement SHALL
take effect on the next deploy or start.
Repeated enable SHALL converge to one container for the saved definition.

#### Scenario: Enable on a running deployment is idempotent

- **WHEN** the user enables the same sidecar twice on a running deployment
- **THEN** one sidecar container is running and available after both commands

#### Scenario: Enable waits for the deployment to start

- **WHEN** a user enables a sidecar on an initialized or stopped deployment
- **THEN** it is reported as enabled and stopped
- **AND** it starts when deploy or start next starts the database

### Requirement: Start reconciles authoritative definitions

Deployment start SHALL reconcile enabled sidecars, including when the
database is already running. Reconciliation SHALL reuse an unchanged running
container, recreate a changed definition, recreate a container whose resolved
connection values have changed, start missing enabled containers, and remove
owned containers whose definitions were removed.

#### Scenario: Start applies an edited definition

- **WHEN** a user changes a saved sidecar environment value and runs start
  while the database is running
- **THEN** the sidecar exposes the new value and the database remains running

#### Scenario: Start applies rotated connection values

- **WHEN** the database password changes and the user runs start while the
  database is running
- **THEN** the sidecar is replaced with one that receives the current password

#### Scenario: Repeated start retains an unchanged sidecar

- **WHEN** start is repeated with an unchanged running sidecar definition
- **THEN** the same sidecar container remains running

#### Scenario: Start completes an interrupted sidecar operation

- **WHEN** an interrupted operation leaves persisted enablement with a
  missing or partially created sidecar and the user runs start
- **THEN** the saved definition converges to one running sidecar

#### Scenario: Start removes a definition deleted by the user

- **WHEN** a user removes a container entry from `sidecars.yaml` and runs start
- **THEN** that sidecar's owned container and published mappings are removed

### Requirement: Sidecars follow deployment stop and restart

Deployment stop SHALL stop all enabled sidecars and retain their definitions.
The next successful start SHALL restore their services using saved definitions
and current connection values.

#### Scenario: Deployment restart restores enabled sidecars

- **WHEN** the user stops and then starts a deployment with enabled sidecars
- **THEN** those sidecars are stopped during the stopped phase and available
  again after startup with their saved configuration

### Requirement: Containers honor restart policy

Sidecars SHALL use their configured restart policy, defaulting to `OnFailure`.
`OnFailure` SHALL restart a process after a nonzero exit; `Always` SHALL restart
after any process exit; `Never` SHALL leave an exited process stopped. Explicit
deployment stop and sidecar disable SHALL take precedence over automatic
restart.

#### Scenario: Process exit follows the selected policy

- **WHEN** a sidecar process exits with a controlled exit code
- **THEN** its subsequent running state matches the selected restart policy

### Requirement: Disable removes sidecar-owned resources idempotently

Disabling a sidecar SHALL remove its saved definition, container, published
mappings, and sidecar-owned state. Repeated disable SHALL report it disabled.
The database SHALL remain queryable with its existing data after disabling a
sidecar on a running deployment.
User-added and renamed saved entries SHALL support disable by their saved name.
Names already absent from desired state SHALL support cleanup retries.
Disabling while the deployment is not running SHALL record the removal without
changing deployment infrastructure, leaving the next start to withdraw the
endpoint.

#### Scenario: Disable on a stopped deployment leaves it stopped

- **WHEN** the user disables a sidecar whose declared port was opened, while
  the deployment is stopped
- **THEN** the definition is removed and the deployment stays stopped, and the
  next start withdraws the endpoint

#### Scenario: Repeated disable retains database availability

- **WHEN** the user disables a running sidecar twice
- **THEN** it is disabled with its owned resources removed
- **AND** database queries return data created before disable

#### Scenario: Disable honors user-edited names

- **WHEN** a user disables a name added directly to `sidecars.yaml` and retries
  after a cleanup failure
- **THEN** its definition is removed and the retry removes its remaining owned
  resources while the database remains running

### Requirement: Connection values reach containers as environment entries

The launcher SHALL pass a sidecar's resolved environment to its container
through a launcher-owned file readable only by its owner, removed once the
container is created. A value the one-entry-per-line format cannot carry SHALL
be rejected naming the variable, without disclosing the value and without
creating the container.

#### Scenario: Unrepresentable value is rejected

- **WHEN** a resolved connection value contains a line break
- **THEN** enablement fails naming the affected variable, the value does not
  appear in the message, and no container is created

### Requirement: Startup failures identify the sidecar and offer diagnosis

A sidecar that fails to start SHALL report its name and a short cause, with a
call to action to run `exasol sidecar status <name>`. The launcher SHALL record
the full diagnostic with that sidecar and report it from status. Recorded
diagnostics SHALL redact database credentials.

#### Scenario: Failed startup offers the status call to action

- **WHEN** a sidecar fails to start, whether its declared host port is already
  claimed or its image is unavailable
- **THEN** the failure names the sidecar with a short cause and tells the user
  to run `exasol sidecar status <name>`
- **AND** status reports the full diagnostic for that sidecar

### Requirement: Status reports sidecar outcomes

`exasol sidecar status <name>` SHALL report the sidecar's enabled state,
running state, IP and port endpoints, and actionable failure when available.
Text and JSON output SHALL represent the same domain result.
Sidecar operation failures SHALL identify the affected sidecar and cause;
database state SHALL reflect the database's own condition.
The launcher SHALL store advisory reconciliation diagnostics in
`sidecars-state.json`. Current runtime observations SHALL determine running
state and endpoints. Status SHALL report `reconciliation` as
`complete`, `pending`, or `unknown`, separately from container running state.
Status SHALL clear recorded failures when inspection verifies the desired
outcome. An enabled sidecar with a running database SHALL require one running
container matching its saved definition, serving exactly the endpoints it
declares. A disabled sidecar or stopped database SHALL require its owned
containers and served endpoints to be absent. An unreachable container
runtime or an inspection failure SHALL retain recorded failures and report
reconciliation as unknown.
Status SHALL distinguish unresolved `lastOperationError` diagnostics from a
live inspection `error`. Status refresh SHALL preserve runtime resources and
saved definitions while persisting verified failure clearance.

#### Scenario: Status reports running and stopped sidecars

- **WHEN** a user queries a sidecar before and after deployment stop
- **THEN** text and JSON output report its enabled and running state, with
  usable published mappings reported while running

#### Scenario: Sidecar failure is reported separately

- **WHEN** a sidecar fails to start while the database is healthy
- **THEN** the operation reports the sidecar failure and status identifies
  its cause while database status remains ready

#### Scenario: Status refreshes resolved failures

- **WHEN** a recorded operation failure remains after runtime state changes
- **THEN** status reports current runtime state and reconciliation,
  clearing and persisting removal of the failure only when the desired outcome
  is verified
- **AND** pending or unverified outcomes retain their recorded failures
- **AND** inspection preserves runtime resources and desired definitions

### Requirement: Destroy cleans up deployment sidecar resources

Deployment destroy SHALL remove owned sidecar containers, mappings, runtime
state, and the deployment service network.

#### Scenario: Destroy removes deployment service resources

- **WHEN** a user destroys a deployment that has enabled sidecars
- **THEN** its owned sidecar runtime resources and service network are removed
