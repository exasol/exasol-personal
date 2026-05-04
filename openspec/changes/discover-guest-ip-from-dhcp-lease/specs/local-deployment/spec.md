# local-deployment Specification (Delta)

## ADDED Requirements

### Requirement: Guest IP discovery from DHCP lease

The system SHALL discover the guest VM's IPv4 address at runtime from the
host's DHCP lease database rather than relying on a hardcoded address. The
discovered address is the target of the host-side loopback forwarders that
bridge SQL and UI traffic into the guest.

#### Scenario: First boot — lease appears before timeout

- GIVEN a fresh local deployment whose VM has just been started
- WHEN the launcher waits for the guest to obtain a DHCP lease
- THEN it polls the host's DHCP lease database
- AND once a lease is recorded for an address on the bridge, it uses that
  address as the forwarder target

#### Scenario: Stale leases ignored in favour of the most recent

- GIVEN a host whose DHCP lease database contains entries for previously
  stopped VMs in addition to the current VM's lease
- WHEN the launcher discovers the guest IP
- THEN it selects the entry with the most recently issued lease
- AND ignores stale entries with earlier expirations

#### Scenario: Lease never appears within the timeout

- GIVEN the launcher is waiting for the guest to obtain a DHCP lease
- WHEN the configured discovery timeout elapses with no usable lease
- THEN the launcher fails fast with an error that references the lease
  database path
- AND does not silently spin in the database-readiness probe with an
  unreachable forwarder target

## MODIFIED Requirements

### Requirement: Database-readiness probe via forwarded loopback

The system SHALL determine local deployment readiness by probing the database
connection through the host-side loopback forwarder. The forwarder targets
the guest IP discovered from the host's DHCP lease database, not a hardcoded
address.

#### Scenario: Deploy waits for the database to accept connections

- GIVEN the launcher has started the local VM and discovered the guest IP
- WHEN the launcher waits for the deployment to be ready
- THEN it polls a database connection through the forwarded loopback port
- AND it reports the deployment as running once the probe succeeds within the
  configured timeout

#### Scenario: Deploy fails when the runner exits before readiness

- GIVEN the launcher is waiting for the local deployment to become ready
- WHEN the runner process exits before the database accepts a connection
- THEN the launcher fails with a clear error referencing the runner log file
