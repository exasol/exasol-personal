## MODIFIED Requirements

### Requirement: Status reports sidecar outcomes

`exasol sidecar status <name>` SHALL report the sidecar's enabled state,
running state, endpoints, and actionable failure when available.
When a saved definition has been removed but cleanup remains unverified,
status SHALL report its recorded failure.
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
