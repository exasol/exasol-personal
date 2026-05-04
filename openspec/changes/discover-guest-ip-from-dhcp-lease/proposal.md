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

## What Changes

- Add a new helper that reads `/var/db/dhcpd_leases`, picks the entry with
  the most recently-issued lease (highest `lease=` hex timestamp), and
  returns its `ip_address=`.
- Wire the helper into `runner.Run` so the host-side loopback forwarders use
  the discovered IP instead of `defaultGuestIPv4`.
- Poll the lease file after `driver.Start` succeeds — the guest needs a few
  seconds after boot to obtain its lease — with a bounded timeout. If the
  timeout elapses with no lease, fail fast with a clear error rather than
  spin forever in the readiness probe.
- Keep the `defaultGuestIPv4` constant as the documented fallback for hosts
  where `/var/db/dhcpd_leases` is unavailable (non-darwin, restricted
  permissions, file deleted by the user). The fallback preserves the prior
  behavior so we do not regress existing single-VM-per-host setups whose
  guest happens to be at `.2`.
- Reorder `runner.Run` so the loopback forwarders are created **after**
  `driver.Start` succeeds, with the discovered IP already in hand. The
  forwarder API itself does not change.

## Impact

This change unblocks repeatable `exasol install local` runs across destroys,
reboots, and any sequence that causes VZ NAT to assign a non-`.2` lease to
the guest. After this change:

- `exasol install local` works regardless of which IP VZ NAT assigns the
  guest, as long as bootpd records the lease in `/var/db/dhcpd_leases`.
- `exasol destroy` followed by another `exasol install local` works on the
  same host without manual edits to launcher constants.
- Multiple deployments per host remain a single-VM-per-host design (the
  most-recent-lease heuristic does not match by deployment identity); a
  future change can plumb the VM's MAC address through `MachineConfig` if
  multi-VM-per-host becomes a requirement.

Out of scope:

- MAC-based lease matching. Today VZ generates a random locally-administered
  MAC inside the framework that the launcher does not track. Plumbing the
  MAC through `MachineConfig` is a follow-up; the most-recent-lease
  heuristic is sufficient for the current single-VM-per-host product
  semantics.
- Guest-side static IP assignment. We continue to rely on bootpd's NAT and
  DHCP rather than configuring the guest with a known address.
- Lease file management or cleanup. macOS owns that file; the launcher only
  reads it.

Impacted capability areas:

- local deployment lifecycle (forwarder target resolution)

Impacted code areas:

- `internal/localruntime/runner.go` (forwarder ordering, discovered IP
  replaces hardcoded constant)
- New `internal/localruntime/dhcp.go` (lease-file reader, pure Go, no
  platform build tags — file simply doesn't exist on non-darwin and the
  reader returns the timeout error after polling)
- Tests under those packages
