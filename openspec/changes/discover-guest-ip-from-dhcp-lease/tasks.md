# Tasks: Discover Guest IP From DHCP Lease

## Phase 1: Lease database parser

- [x] Add `internal/localruntime/dhcp.go` (pure Go, no platform build tags):
  - Constant or var for the lease-database path
    (`/var/db/dhcpd_leases`), overridable for tests.
  - Function `parseDHCPLeases(data []byte) []dhcpLease` that splits the
    file into `{ ... }` blocks and extracts `ip_address` and `lease`
    (parsed as a hex `int64` Unix timestamp). Skip blocks missing either
    field. Tolerate unknown extra fields.
  - Function `selectMostRecentLease(leases []dhcpLease) (dhcpLease, bool)`
    that returns the entry with the highest `lease` value, or
    `_, false` if the slice is empty.
- [x] Add a sentinel error `ErrLeaseDiscoveryTimeout` for the bounded
      polling case.

## Phase 2: Discovery helper with timeout

- [x] Function `(r *Runtime) DiscoverGuestIPv4(ctx context.Context,
      timeout time.Duration) (string, error)` that:
  - Polls the lease database every 500 ms until the file contains at
    least one parseable lease.
  - On success, returns the most-recent-lease IP.
  - On timeout (or if the file never appears), returns
    `ErrLeaseDiscoveryTimeout` wrapping a message that references the
    lease-database path.

## Phase 4: Runner wiring

- [x] In `internal/localruntime/runner.go`:
  - Move forwarder creation to **after** `driver.Start` (currently
    forwarders start first; they need the discovered IP).
  - After `driver.Start` succeeds, call
    `r.DiscoverGuestIPv4(ctx, 60s)`.
  - Use the returned IP for both forwarders.
  - On `ErrLeaseDiscoveryTimeout` or any other discovery error, return
    the error so `local_backend.Deploy` surfaces it via the deployment
    log rather than spinning forever.
  - Remove the `defaultGuestIPv4` constant — discovery is now mandatory
    on the supported platform (darwin/arm64). Non-darwin builds remain
    gated by `validateLocalHostPlatform`.

## Phase 5: Tests

- [x] In `internal/localruntime/dhcp_test.go`:
  - Parser handles a single lease.
  - Parser handles multiple leases and returns the most recent.
  - Parser ignores blocks missing `ip_address` or `lease`.
  - Parser tolerates unknown fields.
  - `selectMostRecentLease` on empty input returns `_, false`.
- [x] Discovery helper test:
  - File initially empty, then a valid block is written part-way through;
    helper returns the IP from that block.
  - File never gains a valid block within the timeout; helper returns
    `ErrLeaseDiscoveryTimeout`.
- [x] Runner test (extend the existing watchStopRequest test if helpful):
  - Stub the discovery helper or set `dhcpLeaseDatabasePath` to a temp
    file with a fixture lease, assert the forwarder is configured with
    that IP.

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
