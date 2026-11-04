## MODIFIED Requirements

### Requirement: Existing deployments can join sidecar networking

New local database containers SHALL use the deployment bridge before
sidecars are enabled when the host supports it. An existing bridge-backed
database SHALL gain service-name connectivity while preserving its active SQL
service. For a running database in a non-bridge network mode, enable SHALL save
the sidecar definition and report it enabled with `restartRequired: true`. Text
output SHALL include a call to action to run `exasol stop` followed by
`exasol start`. JSON output SHALL expose the restart requirement as structured
data.

#### Scenario: Enabling joins an existing running database

- **WHEN** the first sidecar is enabled on a running bridge-backed deployment
- **THEN** the sidecar can reach `database` and the existing database process
  continues serving SQL through its published endpoint

#### Scenario: Existing non-bridge deployment waits for explicit restart

- **WHEN** a sidecar is enabled on a running non-bridge deployment
- **THEN** its definition is saved and status reports enabled, stopped, and
  restart required while the existing database continues serving SQL
- **AND** text output provides the stop/start call to action and JSON output
  exposes `restartRequired: true`
- **AND** the next explicit stop/start activates the bridge and sidecar

## ADDED Requirements

### Requirement: Hosts without service network support keep the database running

Start SHALL start the database without the deployment bridge when starting it
on the bridge fails with Podman's nftables error. Enabling a sidecar on such a running deployment SHALL fail without
saving the sidecar and keep the database running. Starting such a deployment
with an enabled sidecar SHALL start the database and report each enabled
sidecar as failed, as for any other sidecar start failure, and status SHALL
report each enabled sidecar as failed. These failures SHALL state that Podman
could not set up the deployment network. On Windows and inside WSL, the text
output of enable and start SHALL include a call to action naming WSL older than
2.7 as the usual cause and to run `wsl --update`, then `exasol stop` and
`exasol start`. On other hosts, the call to action SHALL name the kernel's
missing nftables support as the usual cause and link to the troubleshooting
documentation. A later start that succeeds on the bridge SHALL
make sidecars available again.

#### Scenario: A deployment without sidecars starts on WSL 2.6

- **WHEN** a local Podman deployment without sidecars is installed or started
  on WSL 2.6
- **THEN** the command succeeds and the database serves SQL

#### Scenario: Enable on WSL 2.6 reports the WSL requirement

- **WHEN** a sidecar is enabled on a running Podman deployment on WSL 2.6
- **THEN** the command fails stating that Podman could not set up the
  deployment network, with a call to action naming WSL older than 2.7 and to
  run `wsl --update`, `exasol stop` and `exasol start`, and the database
  keeps serving SQL
- **AND** the sidecar is not enabled

#### Scenario: Start with an enabled sidecar on WSL 2.6 reports the sidecar failed

- **WHEN** a Podman deployment with an enabled sidecar is started on WSL 2.6
- **THEN** the database serves SQL, the start fails reporting the sidecar with
  the same failure and call to action, and status reports the sidecar failed
  with the same failure

#### Scenario: Sidecars become available after the host is fixed

- **WHEN** WSL is updated to 2.7 or later, or Podman is switched to iptables,
  and the deployment is stopped and started
- **THEN** enabling a sidecar succeeds without a further restart
