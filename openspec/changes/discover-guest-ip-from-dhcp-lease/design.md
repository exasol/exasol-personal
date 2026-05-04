# Design: Discover Guest IP From DHCP Lease

## Overview

Replace the hardcoded `defaultGuestIPv4 = "192.168.64.2"` in
`runner.go` with a runtime read of macOS bootpd's lease database at
`/var/db/dhcpd_leases`, picking the entry with the most recently issued
lease as the guest IP for the loopback forwarders.

## Lease file format

`/var/db/dhcpd_leases` is a plaintext file written by the macOS bootpd
process backing VZ's NAT. Each lease is a brace-delimited block of
`key=value` lines:

```
{
        name=exasol-vm
        ip_address=192.168.64.3
        hw_address=1,86:d0:94:72:c8:a8
        identifier=1,86:d0:94:72:c8:a8
        lease=0x683783b0
}
```

- `ip_address=` is a plain IPv4 dotted-quad.
- `hw_address=` is `<htype>,<MAC>`.
- `lease=` is a hex Unix timestamp of the lease expiration.

Old leases for now-stopped VMs accumulate in the file with their original
expiration timestamps. New leases for currently-running VMs always have a
later expiration than stale ones (because bootpd issues them later).

The file is world-readable on stock macOS (`-rw-r--r--`); the launcher does
not need elevated privileges.

## Discovery algorithm

1. Read `/var/db/dhcpd_leases`. If missing or unreadable, return the
   fallback IP and a sentinel error.
2. Tokenize into `{ ... }` blocks.
3. For each block, parse `ip_address` and `lease`. Skip blocks missing
   either field.
4. Pick the block with the highest `lease` value. Return its `ip_address`.
5. If no usable block was parsed, return the fallback IP and a sentinel
   error.

The "most recent lease" heuristic correctly handles the typical
`destroy → install` cycle: the new VM's lease has a higher expiration than
any stale entry, so it wins.

## Polling and timeout

The guest needs a few seconds after `driver.Start` to bring up its network
interface and complete its DHCP handshake. The launcher polls the lease
file:

- Cadence: every 500 ms.
- Timeout: 60 s. (Boot + DHCP normally completes well under 30 s; 60 s
  leaves slack for cold caches or busy CI hosts.)

If the timeout elapses with no lease for any IP in the bridge's subnet,
the runner returns a clear error referencing both the timeout and the
lease-file path. The launcher's existing readiness probe will then surface
that error rather than silently spinning.

## Runner ordering

`StartLoopbackForwarder`'s signature does not change. The discovery call
runs in `runner.Run` between `driver.Start` and the forwarder creation,
so the discovered IP is available as a plain string when the forwarder
is built. The order in `runner.Run` becomes:

1. EnsureRoot, EnsurePayloadSelected, ResetControlState, PrepareGuest,
   Controller.Ensure, LoadState, WriteRunnerPID — unchanged.
2. `driver.Start(ctx, guest.Machine)` — the VM boots, the guest brings
   up its network interface, the guest's DHCP client gets a lease.
3. `r.DiscoverGuestIPv4(ctx, 60s)` — poll `/var/db/dhcpd_leases` for the
   most recent lease.
4. `StartLoopbackForwarder` for SQL and UI ports, targeting the
   discovered IP. (Forwarders previously started **before** Start; they
   move **after** so the IP is known.)
5. Stop-watcher goroutine.
6. `driver.Wait` — block until shutdown.

The discovery call's bounded 60 s timeout prevents a wedged guest from
hanging the runner indefinitely. On timeout, the runner returns
`ErrLeaseDiscoveryTimeout` wrapping the lease-database path so the
launcher's deployment log surfaces the failure clearly.

## Cross-platform compatibility

`/var/db/dhcpd_leases` is darwin-specific. To keep Linux CI and
cross-compilation working:

- `internal/localruntime/dhcp.go` is gated by `//go:build darwin`.
- `internal/localruntime/dhcp_other.go` is gated by `//go:build !darwin`
  and returns the fallback IP plus a sentinel "not implemented on this
  platform" error.

The runner code calls into the platform-agnostic helper and decides what
to do with the sentinel: on darwin, treat as fatal after the timeout; on
non-darwin, the local runtime is already gated by
`validateLocalHostPlatform()` (Apple Silicon + macOS 13+) so this code
path is unreachable in production. The platform-stub keeps build-time
compile errors away on Linux.

## Observable behavior

Before:

- `exasol install local` after `exasol destroy` on the same host: forwarders
  dial `.2`; if VZ assigned `.3` to the new VM, dial fails with
  "context deadline exceeded"; readiness probe times out at 30 minutes;
  user must manually edit `defaultGuestIPv4`.

After:

- `exasol install local` after `exasol destroy` on the same host: VM
  boots, guest DHCP completes, launcher reads the lease file, forwarders
  dial the discovered IP, readiness probe succeeds within seconds.
- Lease file unreadable on darwin: launcher fails fast with a message
  referencing the lease-file path and the fallback constant.
- Non-darwin host: build still compiles; the fallback path is unreachable
  in practice because `validateLocalHostPlatform()` rejects the host
  earlier.

## Risks

- The guest's DHCP client takes longer than 60 s to complete. Possible on
  very slow hosts or pathological retry storms. Mitigation: timeout is
  bounded and the error message is clear; the user can retry.
- macOS rotates or restructures `/var/db/dhcpd_leases` in a future
  release. Low likelihood (file format has been stable for a decade);
  mitigation is a unit-tested parser that gracefully ignores unknown
  fields.
- Multiple unrelated VMs on the same host (e.g., the user is running
  Lima, Multipass, or a second exasol deployment in parallel). The
  most-recent-lease heuristic could pick the wrong VM. Documented as a
  single-VM-per-host limitation; MAC-matching is a follow-up.
- Permissions: a sandboxed macOS environment may deny read access to
  `/var/db/dhcpd_leases`. The launcher already requires the
  `com.apple.security.virtualization` entitlement which is incompatible
  with strict App Sandbox; this is the same gating concern as the rest
  of the local backend.
