# Tasks: Discover Guest IP From DHCP Lease

## Phase 1: Lease database parser

- [x] Add `internal/localruntime/dhcp.go` (pure Go, no platform build tags):
  - Constant or var for the lease-database path
    (`/var/db/dhcpd_leases`), overridable for tests.
  - Function `parseDHCPLeases(data []byte) []dhcpLease` that splits the
    file into `{ ... }` blocks and extracts `ip_address`, `hw_address`
    in either of two formats (`1,<MAC>` legacy, or
    `ff,<IAID>:<DUID>` RFC 4361 — captured as IAIDSuffix, the first
    4 bytes after the comma), and `lease` (parsed as a hex `int64`).
    Skip blocks missing any of the three. Tolerate unknown extra fields.
  - Function `selectLeaseByMAC(leases []dhcpLease, canonicalMAC
    string) (dhcpLease, bool)` that matches either:
      * `lease.HWAddress` equals the full canonical MAC (legacy
        format), or
      * `lease.IAIDSuffix` equals the last 4 bytes of the canonical
        MAC (RFC 4361 format).
  - Helpers `canonicalizeMAC`, `parseLeaseHWAddress`,
    `parseIAIDSuffix`, and `lastFourBytesOfMAC` to handle the two
    formats and compare via canonical lowercase forms.
- [x] Add a sentinel error `ErrLeaseDiscoveryTimeout` for the bounded
      polling case.

## Phase 1.5: Plumb MAC from launcher to driver

- [x] In `internal/localruntime/vm/driver.go` add `MACAddress string`
      to `MachineConfig`.
- [x] In `internal/localruntime/vm/driver_darwin_arm64.go`:
  - Update `buildNetworkDevices(requestedMAC string)` to take the
    requested MAC.
  - Add `buildMACAddress(requested string) (*vz.MACAddress, error)`
    that returns `vz.NewMACAddress(net.ParseMAC(requested))` when
    requested is non-empty, falling back to
    `vz.NewRandomLocallyAdministeredMACAddress()` only when empty
    (preserves test paths that don't supply a MAC).
- [x] In `internal/localruntime/guest.go`:
  - Add `MACAddress string` to `GuestConfig`.
  - Add `generateLocallyAdministeredMAC() (string, error)` that fills
    a 6-byte buffer via `crypto/rand`, sets the LAA bit (`0x02`) and
    clears the multicast bit (`0x01`) of byte 0, and returns the
    canonical lowercase colon form.
  - Have `PrepareGuest` generate the MAC, set it on both
    `MachineConfig.MACAddress` and `GuestConfig.MACAddress`.

## Phase 2: Discovery helper with timeout

- [x] Function `(r *Runtime) DiscoverGuestIPv4(ctx context.Context,
      mac string, timeout time.Duration) (string, error)` that:
  - Canonicalizes the supplied MAC via `net.ParseMAC`.
  - Polls the lease database every 500 ms until a lease matching the
    canonical MAC appears.
  - On success, returns that lease's IP.
  - On timeout (or if the file/MAC never appears), returns
    `ErrLeaseDiscoveryTimeout` wrapping a message that references the
    lease-database path AND the requested MAC.

## Phase 4: Runner wiring

- [x] In `internal/localruntime/runner.go`:
  - Move forwarder creation to **after** `driver.Start` (currently
    forwarders start first; they need the discovered IP).
  - After `driver.Start` succeeds, call
    `r.DiscoverGuestIPv4(ctx, guest.MACAddress, 60s)`.
  - Log the discovered IP and the MAC it was matched against.
  - Use the returned IP for both forwarders.
  - On `ErrLeaseDiscoveryTimeout` or any other discovery error, return
    the error so `local_backend.Deploy` surfaces it via the deployment
    log rather than spinning forever.
  - Remove the `defaultGuestIPv4` constant — discovery is now mandatory
    on the supported platform (darwin/arm64). Non-darwin builds remain
    gated by `validateLocalHostPlatform`.

## Phase 5: Tests

- [x] In `internal/localruntime/dhcp_test.go`:
  - Parser captures `ip_address`, `hw_address` (canonicalized), and
    `lease` from a single block.
  - Parser ignores blocks missing any of the three required fields.
  - Parser tolerates unknown fields.
  - `selectLeaseByMAC` returns the matching entry even when other
    blocks have later `lease=` values.
  - `selectLeaseByMAC` returns `_, false` for an unknown MAC.
  - `selectLeaseByMAC` returns `_, false` for empty input.
  - `canonicalizeMAC` accepts uppercase, hyphenated, and Cisco-dotted
    forms; emits canonical lowercase-colon form.
- [x] Discovery helper tests:
  - Lease file appears mid-poll with a matching MAC; helper returns the
    IP.
  - Lease file present but no matching MAC; helper times out with
    `ErrLeaseDiscoveryTimeout`.
  - Lease file never appears; helper times out with
    `ErrLeaseDiscoveryTimeout`.
- [x] In `internal/localruntime/mac_test.go`:
  - `generateLocallyAdministeredMAC` produces a parseable MAC with
    LAA bit set and multicast bit clear.
  - Two consecutive calls produce different MACs.
- [x] In `internal/localruntime/guest_test.go`:
  - `PrepareGuest` populates both `GuestConfig.MACAddress` and
    `MachineConfig.MACAddress`, and they agree.

## Phase 6: Build and lint

- [x] `task lint-golang` clean (no new issues vs. main).
- [x] `task tests-unit` clean.
- [x] `task build` succeeds on Linux (uses dhcp_other.go stub).
- [ ] On a darwin/arm64 host, `task build` succeeds.

## Phase 7: Manual smoke test (developer-only, not CI)

Performed on a Mac (macOS 13+, Apple Silicon):

- [ ] Run `exasol install local` against a fresh deployment dir. Confirm
      success.
- [ ] Run `exasol destroy`. Run `exasol install local` again. Confirm
      success without any manual launcher edits or rebuilds.
- [ ] Inspect `/var/db/dhcpd_leases` between runs to confirm the most
      recent lease is the one the launcher targeted.
- [ ] Confirm the launcher fails fast (within ~60 s) with a clear error
      message if `/var/db/dhcpd_leases` is removed or made unreadable.

## Phase 8: Spec deltas

- [x] Confirm `openspec/changes/discover-guest-ip-from-dhcp-lease/specs/local-deployment/spec.md`
      matches the implementation.
- [x] Run `openspec validate discover-guest-ip-from-dhcp-lease`.
