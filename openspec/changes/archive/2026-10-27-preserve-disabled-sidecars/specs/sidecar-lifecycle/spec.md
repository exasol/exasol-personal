## RENAMED Requirements

- FROM: `### Requirement: Disable removes sidecar-owned resources idempotently`
- TO: `### Requirement: Disable keeps saved definitions and removal deletes them`

## MODIFIED Requirements

### Requirement: Start reconciles authoritative definitions

Deployment start SHALL reconcile enabled sidecars, including when the
database is already running. Reconciliation SHALL reuse an unchanged running
container, recreate a changed definition, recreate a container whose resolved
connection values have changed, start missing enabled containers, and remove
owned containers whose definitions were removed or disabled.

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

#### Scenario: Start removes a sidecar disabled by the user

- **WHEN** a user sets `enabled: false` on a running sidecar's entry in
  `sidecars.yaml` and runs start
- **THEN** that sidecar's owned container and published mappings are removed
  and its saved definition remains

### Requirement: Disable keeps saved definitions and removal deletes them

Disabling a sidecar SHALL save `enabled: false` on its entry and remove its
container, published mappings, and sidecar-owned state, keeping the rest of its
saved definition. Enabling it again SHALL use that saved definition.
`exasol sidecar disable <name> --remove` SHALL remove the same resources and
delete the entry from `sidecars.yaml`, so a later enable of a catalog name
saves its catalog template again. Repeating either form SHALL succeed and
report that there is nothing to remove. A name the deployment has not saved
SHALL report that there is nothing to remove.
The database SHALL remain queryable with its existing data after disabling a
sidecar on a running deployment.
User-added and renamed saved entries SHALL support disable by their saved name.
Names already absent from desired state SHALL support cleanup retries.
Disabling while the deployment is not running SHALL record the change without
changing deployment infrastructure, leaving the next start to withdraw the
endpoint.

#### Scenario: Re-enabling restores the edited definition

- **WHEN** the user edits a running sidecar's host port, disables it, and
  enables it again
- **THEN** after disable its container and mapping are removed and
  `sidecars.yaml` keeps the edited definition with `enabled: false`
- **AND** after enable the sidecar serves on the edited host port

#### Scenario: Removal returns a catalog sidecar to its template

- **WHEN** the user edits a catalog sidecar's saved definition, runs
  `exasol sidecar disable <name> --remove`, and enables it again
- **THEN** after removal `sidecars.yaml` has no entry for that name
- **AND** after enable the entry matches the catalog template

#### Scenario: Repeated removal reports nothing to remove

- **WHEN** the user runs `exasol sidecar disable <name> --remove` twice
- **THEN** the second command succeeds and reports that there is nothing to
  remove

#### Scenario: Disable on a stopped deployment leaves it stopped

- **WHEN** the user disables a sidecar whose declared port was opened, while
  the deployment is stopped
- **THEN** the entry is saved as disabled and the deployment stays stopped, and
  the next start withdraws the endpoint

#### Scenario: Repeated disable retains database availability

- **WHEN** the user disables a running sidecar twice
- **THEN** the second disable reports that there is nothing to remove
- **AND** it remains disabled with its owned resources removed
- **AND** database queries return data created before disable

#### Scenario: Disable honors user-edited names

- **WHEN** a user disables a name added directly to `sidecars.yaml` and retries
  after a cleanup failure
- **THEN** its entry is saved as disabled and the retry removes its remaining
  owned resources while the database remains running
