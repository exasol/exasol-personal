# Design: Discover Guest IP From DHCP Lease

## Overview

Replace the hardcoded `defaultGuestIPv4 = "192.168.64.2"` in
`runner.go` with a runtime read of macOS bootpd's lease database at
`/var/db/dhcpd_leases`. The launcher generates the VM's MAC address
itself before booting and uses that MAC as the lookup key against
`hw_address=` entries in the lease database, returning the IP for the
exact VM it just started.

## Lease file format

`/var/db/dhcpd_leases` is a plaintext file written by the macOS bootpd
process backing VZ's NAT. Each lease is a brace-delimited block of
`key=value` lines. The `hw_address=` field uses one of two formats
depending on what client identifier the guest's DHCP client supplied:

### Legacy format (`htype=1`)

```
{
        name=exasol-vm
        ip_address=192.168.64.3
        hw_address=1,86:d0:94:72:c8:a8
        identifier=1,86:d0:94:72:c8:a8
        lease=0x683783b0
}
```

`hw_address=1,<MAC>` — the literal 6-byte hardware address. Older
macOS versions and clients that send a hardware-address-only client
identifier produce this form.

### RFC 4361 client identifier (`htype=255`)

Modern macOS (15.7+) running VZ NAT against an Alpine guest produces
this form:

```
{
        name=exasol-vm
        ip_address=192.168.64.5
        hw_address=ff,2f:7d:f5:3a:0:1:0:1:31:86:54:f0:52:54:0:12:34:56
        identifier=ff,2f:7d:f5:3a:0:1:0:1:31:86:54:f0:52:54:0:12:34:56
        lease=0x69f840df
}
```

The structure after `ff,` is:

- **IAID** (4 bytes): `2f:7d:f5:3a`. RFC 4361 says clients SHOULD use
  the persistent interface identifier as the IAID; Linux's dhclient and
  Alpine's busybox-udhcpc both use the **last 4 bytes of the MAC**. So
  the IAID equals the suffix of the VM's MAC `62:a8:2f:7d:f5:3a`.
- **DUID** (variable length): `0:1:0:1:31:86:54:f0:52:54:0:12:34:56`.
  Type=DUID-LLT (`00:01`), HW=Ethernet (`00:01`), Time=`31:86:54:f0`,
  Link-layer address=`52:54:00:12:34:56`. The link-layer address
  identifies the host's vmnet bridge, **not** the VM, and is
  identical across all entries on the same host.

### Lookup strategy

Because the running VM's identifier appears in the lease as the IAID
suffix (last 4 bytes of MAC), the matcher computes both the full MAC
and the 4-byte suffix from the launcher-supplied MAC, then matches
each `hw_address=` entry against whichever form it presents.

### Other fields

- `ip_address=` is a plain IPv4 dotted-quad.
- `lease=` is a hex Unix timestamp of the lease expiration.

Old leases for now-stopped VMs accumulate in the file with their
original expiration timestamps. The MAC-based lookup ignores them
because their IAIDs are different.

The file is world-readable on stock macOS (`-rw-r--r--`); the launcher
does not need elevated privileges.

## MAC generation

The launcher generates the VM's MAC address in `PrepareGuest` using
`crypto/rand` to fill 6 bytes, then sets the locally-administered bit
(`0x02`) and clears the multicast bit (`0x01`) of the first octet. The
canonical lowercase form (`aa:bb:cc:dd:ee:ff`) is stored on
`MachineConfig.MACAddress` and `GuestConfig.MACAddress`. The VZ driver,
in `buildNetworkDevices`, parses the supplied MAC via `net.ParseMAC` and
constructs the `vz.MACAddress` from it via `vz.NewMACAddress` instead of
calling `vz.NewRandomLocallyAdministeredMACAddress`. If
`MachineConfig.MACAddress` is empty (only test paths today), the driver
falls back to the random constructor — production code always supplies
a MAC.

## Discovery algorithm

1. Canonicalize the supplied MAC via `net.ParseMAC(mac).String()` so
   comparisons are case- and separator-insensitive.
2. Compute the 4-byte suffix of the canonical MAC for matching against
   RFC 4361 IAIDs (`62:a8:2f:7d:f5:3a` → `2f:7d:f5:3a`).
3. Read `/var/db/dhcpd_leases`. If missing or unreadable, treat as
   "lease not yet available" and continue polling.
4. Tokenize into `{ ... }` blocks.
5. For each block, parse `ip_address`, `hw_address`, and `lease`. Skip
   blocks missing any of the three. For `hw_address=`, dispatch on the
   `<htype>,` prefix:
   - `1,<MAC>` → canonicalize the trailing MAC into `HWAddress`.
   - `ff,<IAID>:<DUID>` → take the first 4 octets after the comma as
     the IAID, store as `IAIDSuffix`.
   - Other htypes → skip the block.
6. Find the lease whose `HWAddress` equals the full MAC, or whose
   `IAIDSuffix` equals the 4-byte MAC suffix. Return its `ip_address`.
7. If no matching lease is present, treat as "lease not yet available"
   and continue polling.

Matching by MAC (or its IAID suffix) is deterministic. Each VM has a
unique MAC, so there is exactly one correct lease entry to return.
Stale entries from prior VMs have different MACs (and therefore
different IAIDs) and are silently ignored, regardless of how recent
their `lease=` expirations are.

## Polling and timeout

The guest needs a few seconds after `driver.Start` to bring up its network
interface and complete its DHCP handshake. The launcher polls the lease
file:

- Cadence: every 500 ms.
- Timeout: 60 s. (Boot + DHCP normally completes well under 30 s; 60 s
  leaves slack for cold caches or busy CI hosts.)

If the timeout elapses without a matching lease appearing for the
supplied MAC, the runner returns a clear error referencing the MAC,
the timeout, and the lease-file path. The launcher's existing
readiness probe surfaces that error rather than silently spinning.

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

`/var/db/dhcpd_leases` is darwin-specific, but the lease parser is pure
Go with no platform-specific syscalls. We keep `dhcp.go` unrestricted
by build tags. On non-darwin hosts the file simply does not exist, so
`os.ReadFile` returns `os.ErrNotExist`, the helper treats that as
"lease not yet available," and the polling loop times out.
`validateLocalHostPlatform` rejects non-darwin hosts before
`runner.Run` is reached in production, so the timeout path is
unreachable in practice. This keeps Linux CI builds compiling without
a separate platform stub.

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

- The guest's DHCP client takes longer than 60 s to complete. Possible
  on very slow hosts or pathological retry storms. Mitigation: timeout
  is bounded and the error message is clear; the user can retry.
- macOS rotates or restructures `/var/db/dhcpd_leases` in a future
  release. The file moved from `htype=1` to `htype=ff` (RFC 4361)
  representation between macOS versions; the parser supports both.
  Further format drift would require an additional parser branch; the
  unit-tested fixtures make that easy to extend.
- A guest DHCP client uses an IAID that is **not** the last 4 bytes of
  its MAC. RFC 4361 leaves the IAID opaque; clients are free to choose.
  Linux dhclient and Alpine's udhcpc do use the MAC-suffix convention
  and that is what we observe with our shipped images. If a different
  guest distribution is used in the future and breaks this assumption,
  the fix is to parse the DUID's link-layer address rather than the
  IAID, or to plumb the IAID separately. Documented as an
  Alpine-image-specific assumption.
- Multiple unrelated VMs on the same host (Lima, Multipass, a second
  Exasol deployment). The MAC-matching scheme correctly ignores those
  unrelated VMs because their MACs are different. The remaining
  concurrency limit is launcher-side state isolation, not IP discovery.
- Permissions: a sandboxed macOS environment may deny read access to
  `/var/db/dhcpd_leases`. The launcher already requires the
  `com.apple.security.virtualization` entitlement which is incompatible
  with strict App Sandbox; this is the same gating concern as the rest
  of the local backend.
- The locally-administered MAC the launcher generates collides with
  another VM's MAC on the same bridge. Probability is ~1 in 2^46 per
  pair; effectively zero for a developer Mac with a handful of VMs.
