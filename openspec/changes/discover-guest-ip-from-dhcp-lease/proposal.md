# Proposal: Discover Guest IP From DHCP Lease

## Why

The local runtime currently hardcodes `defaultGuestIPv4 = "192.168.64.2"` in
[`runner.go`](../../../internal/localruntime/runner.go) for the host-side
loopback forwarders that bridge SQL and UI traffic from the host into the
guest. This was carried over from the kernel-boot path on the assumption that
Apple Virtualization.framework's NAT consistently assigns `.2` to the first
guest on `bridge100`.

Smoke testing on the EFI-boot path has shown this assumption does not hold in
practice. macOS's bootpd (the DHCP server backing VZ NAT) caches leases at
`/var/db/dhcpd_leases`. Once a lease has been issued for `.2` to a previous
VM, the next VM gets `.3`, then `.4`, and so on. After a few `exasol install
local` / `exasol destroy` cycles the launcher's hardcoded `.2` no longer
matches where the active VM actually lives, the loopback forwarders dial an
unreachable address, and `verifyDatabaseConnectionFn` times out even though
the database is up and listening.

The hardcoded value also makes the launcher fragile across machine reboots,
multiple deployments per host, and parallel local installations.

The fix is to read the active guest's IPv4 address from
`/var/db/dhcpd_leases` at runtime, after the VM has booted and the guest's
DHCP client has obtained a lease, and use that address for the forwarders.

The launcher generates the VM's MAC address itself (a random
locally-administered unicast address) before the VM boots, passes it to the
VZ driver, and uses the same MAC as the lookup key when reading the lease
database. The first heuristic considered — picking the lease with the most
recent expiration — proved unreliable: stale entries from prior VMs can
have later expirations than freshly issued leases when the previous VM was
running long enough to renew its lease multiple times. Matching by MAC is
deterministic because each VM's MAC is unique within the host's bridge.

## What Changes

- Add a new helper that reads `/var/db/dhcpd_leases`, finds the entry whose
  `hw_address=` matches the launcher-supplied MAC, and returns its
  `ip_address=`.
- Generate the VM's MAC address in the launcher (random
  locally-administered unicast) and pass it to the VZ driver via
  `MachineConfig.MACAddress`. The driver uses this MAC instead of calling
  `vz.NewRandomLocallyAdministeredMACAddress()` itself, so the launcher
  knows the running VM's MAC for lease lookup.
- Wire the helper into `runner.Run` so the host-side loopback forwarders use
  the discovered IP instead of a hardcoded address. The helper takes the
  generated MAC as input.
- Poll the lease file after `driver.Start` succeeds — the guest needs a few
  seconds after boot to obtain its lease — with a bounded timeout. If the
  timeout elapses with no matching lease, fail fast with a clear error
  rather than spin forever in the readiness probe.
- Reorder `runner.Run` so the loopback forwarders are created **after**
  `driver.Start` succeeds, with the discovered IP already in hand. The
  forwarder API itself does not change.
- Remove the `defaultGuestIPv4 = "192.168.64.2"` constant. Discovery is
  always required on the supported darwin/arm64 platform; the existing
  `validateLocalHostPlatform` check gates non-darwin builds out of this
  code path.

## Impact

This change unblocks repeatable `exasol install local` runs across destroys,
reboots, and any sequence that causes VZ NAT to assign a non-`.2` lease to
the guest. After this change:

- `exasol install local` works regardless of which IP VZ NAT assigns the
  guest, as long as bootpd records the lease in `/var/db/dhcpd_leases`.
- `exasol destroy` followed by another `exasol install local` works on the
  same host without manual edits to launcher constants, regardless of
  what stale leases for prior VMs remain in the DHCP database.

Out of scope:

- Concurrent multi-VM-per-host. The MAC-matching scheme is correct for
  multiple VMs because each gets a distinct MAC, but the launcher's
  per-deployment state (deployment dir, port allocation) still assumes
  one local deployment at a time. True concurrent multi-VM is a separate
  state-isolation effort.
- Guest-side static IP assignment. We continue to rely on bootpd's NAT
  and DHCP rather than configuring the guest with a known address.
- Lease file management or cleanup. macOS owns that file; the launcher
  only reads it.

Impacted capability areas:

- local deployment lifecycle (forwarder target resolution, MAC
  assignment)

Impacted code areas:

- `internal/localruntime/vm/driver.go` (`MachineConfig.MACAddress`)
- `internal/localruntime/vm/driver_darwin_arm64.go` (use the supplied
  MAC in `buildNetworkDevices`)
- `internal/localruntime/guest.go` (generate the MAC in `PrepareGuest`,
  populate `GuestConfig.MACAddress` and `MachineConfig.MACAddress`)
- `internal/localruntime/runner.go` (forwarder ordering; discovered IP
  replaces hardcoded constant; pass `guest.MACAddress` to discovery)
- New `internal/localruntime/dhcp.go` (lease-file reader, pure Go, no
  platform build tags — file simply doesn't exist on non-darwin and
  the reader returns the timeout error after polling)
- Tests under those packages
