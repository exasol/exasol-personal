## Purpose

Let users manage enabled sidecars with their deployment and reconcile saved
configuration while keeping service failures and cleanup understandable.

## ADDED Requirements

### Requirement: Cloud hosts share one desired definition

Cloud deployments SHALL apply the saved definition to every node. Failed
operations on one node SHALL allow attempts on the remaining nodes. Retrying
SHALL reconcile every node against the same saved definition.

#### Scenario: Cloud enablement precedes provisioning

- **WHEN** the user enables a sidecar on an initialized cloud deployment
- **THEN** its definition is saved and status reports enabled with empty hosts

#### Scenario: Hosts reconcile independently after a failure

- **WHEN** sidecar startup fails on one host and succeeds on another
- **THEN** status reports each host's outcome and the definition remains enabled
- **AND** retrying after correcting the failure starts the sidecar on every host

#### Scenario: Disable retries a failed host independently

- **WHEN** disable fails to remove a sidecar on one of several hosts
- **THEN** the definition is disabled and cleanup proceeds on the other hosts
- **AND** retrying disable finishes the remaining cleanup

## MODIFIED Requirements

### Requirement: Sidecars follow deployment stop and restart

Deployment stop SHALL stop all enabled sidecars and retain their definitions.
The next successful start SHALL restore their services using saved definitions
and current connection values.
Cloud sidecars SHALL belong to their node's database service, so a node that
starts that service SHALL start its enabled sidecars and a node that stops it
SHALL stop them.

#### Scenario: Deployment restart restores enabled sidecars

- **WHEN** the user stops and then starts a deployment with enabled sidecars
- **THEN** those sidecars are stopped during the stopped phase and available
  again after startup with their saved configuration

#### Scenario: Cloud node follows its own database service

- **WHEN** a cloud node restarts its database service without a launcher
  command
- **THEN** its enabled sidecars run again with their saved configuration
- **AND** stopping that service leaves them stopped

### Requirement: Status reports sidecar outcomes

`exasol sidecar status <name>` SHALL report deployment-wide enabled state and
a `hosts` array containing each host's name, running state, IP and port
endpoints, and actionable failure when available. Local deployments SHALL use
the host name `local`; cloud deployments SHALL use node names. Text and JSON
output SHALL represent the same domain result.
Sidecar operation failures SHALL identify the affected host, sidecar, and cause;
database state SHALL reflect the database's own condition.
The launcher SHALL store advisory reconciliation diagnostics in
`sidecars-state.json`. Current runtime observations SHALL determine running
state and endpoints. Status SHALL report per-host `reconciliation` as
`complete`, `pending`, or `unknown`, separately from container running state.
Status SHALL clear recorded failures when inspection verifies the desired
outcome. An enabled sidecar with a running database SHALL require one running
container matching its saved definition, serving exactly the endpoints it
declares. A disabled sidecar or stopped database SHALL require its owned
containers and served endpoints to be absent. Unavailable hosts and inspection
failures SHALL retain recorded failures and report reconciliation as unknown.
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
- **THEN** status reports current runtime state and per-host reconciliation,
  clearing and persisting removal of the failure only when the desired outcome
  is verified
- **AND** pending or unverified outcomes retain their recorded failures
- **AND** inspection preserves runtime resources, desired definitions, and
  other hosts' recorded failures
