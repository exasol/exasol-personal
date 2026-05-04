# local-deployment Specification (Delta)

## ADDED Requirements

### Requirement: Guest IP discovery from DHCP lease

The system SHALL discover the guest VM's IPv4 address at runtime from the
host's DHCP lease database by matching the lease entry whose recorded MAC
address equals the launcher-supplied MAC for the running VM. The
discovered address is the target of the host-side loopback forwarders
that bridge SQL and UI traffic into the guest.

#### Scenario: Lease matched by VM MAC address

- GIVEN a fresh local deployment whose VM has just been started with a
  launcher-generated MAC
- WHEN the launcher waits for the guest to obtain a DHCP lease
- THEN it polls the host's DHCP lease database
- AND once a lease whose `hw_address=` matches the VM's MAC is recorded,
  the launcher uses that lease's `ip_address=` as the forwarder target

#### Scenario: Stale leases for prior VMs are ignored

- GIVEN a host whose DHCP lease database contains entries for previously
  stopped VMs in addition to the current VM's lease
- WHEN the launcher discovers the guest IP
- THEN it returns only the lease whose MAC matches the running VM
- AND ignores entries whose MAC differs, regardless of their recorded
  expiration time

#### Scenario: Lease never appears within the timeout

- GIVEN the launcher is waiting for the guest to obtain a DHCP lease
- WHEN the configured discovery timeout elapses with no matching lease
- THEN the launcher fails fast with an error that references the lease
  database path and the requested MAC
- AND does not silently spin in the database-readiness probe with an
  unreachable forwarder target

## MODIFIED Requirements

### Requirement: Database-readiness probe via forwarded loopback

The system SHALL determine local deployment readiness by probing the database
connection through the host-side loopback forwarder. The forwarder targets
the guest IP discovered from the host's DHCP lease database via MAC matching,
not a hardcoded address.

#### Scenario: Deploy waits for the database to accept connections

- GIVEN the launcher has started the local VM and discovered its IP via
  MAC-matched lookup in the DHCP lease database
- WHEN the launcher waits for the deployment to be ready
- THEN it polls a database connection through the forwarded loopback port
- AND it reports the deployment as running once the probe succeeds within the
  configured timeout

#### Scenario: Deploy fails when the runner exits before readiness

- GIVEN the launcher is waiting for the local deployment to become ready
- WHEN the runner process exits before the database accepts a connection
- THEN the launcher fails with a clear error referencing the runner log file
