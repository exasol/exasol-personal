# Design: Replace Kernel + Initrd Boot With EFI Disk Boot

## Overview

Switch the local runtime's boot mechanism from
`vz.NewLinuxBootLoader(kernelPath, WithInitrd, WithCommandLine)` to the vz/v3
EFI bootloader binding reading from a pre-built **generic** Alpine EFI disk
image. The disk image is produced by `exasol-nano-vm` and contains no
Exasol-specific runtime: it has only an OpenRC `exasol-bootstrap` service that
executes `/mnt/host/start.sh` on every boot if the file is present.

The launcher remains the owner of guest execution, but switches its delivery
channel from "kernel command line + initrd-interpreted virtio-fs shares" to "a
single launcher-staged virtio-fs share at `/mnt/host`."

This is a full replacement, not a coexistence. The kernel + initrd code path is
removed.

## Boot path

### Before

`PrepareGuest` builds a `MachineConfig` with `KernelPath`, `InitrdPath`, and a
custom `KernelCommandLine` that the Ubuntu-derived initrd interprets via
`exa_volume=`, `exa_layer_key=`, `exa_sql_port=`, `exa_ui_port=` parameters.

The driver builds the bootloader with `vz.NewLinuxBootLoader`. A separate
writable layer disk is created per deployment for guest persistence. Multiple
virtio-fs shares (`exa-payload`, `exa-provision`, `exa-logs`, `exa-control`)
deliver bootstrap assets and act as the host↔guest control bridge.

### After

`PrepareGuest` builds a `MachineConfig` with:

- A single boot disk (`DiskImagePath`) — the staged copy of the generic Alpine
  EFI image.
- An EFI variable store path (`EFIVarsPath`).
- A single read-write virtio-fs share with tag `hostshare`, source
  `<deploymentDir>/local-runtime/vm/payload-share/`, destination `/mnt/host`.

The driver builds the bootloader with the vz/v3 EFI binding configured against a
per-deployment EFI variable store file so EFI variables persist across boots.

The Alpine image's `exasol-bootstrap` OpenRC service runs on every boot. It
checks for `/mnt/host/start.sh` and, if executable, runs it. The launcher
authors and stages that script.

## Payload share staging

Before each boot, the launcher stages two files into the per-deployment
payload-share directory:

1. **`db.run`** — the cached, version-pinned Exasol installer binary copied from
   the user-cache directory. Staged with mode 0700 so it is executable inside
   the guest. Idempotent on identical content (skip-on-checksum-match).
2. **`start.sh`** — a launcher-owned, embedded shell script (via `//go:embed`)
   that runs inside the guest. Responsibilities:
   - Set the environment expected by `db.run` (e.g., `HOME=/root`).
   - Invoke `/mnt/host/db.run` (or, if a future iteration ships an
     `entrypoint.sh`, invoke that).
   - Detach the DB process so the OpenRC service exits cleanly while the DB
     stays running.

`start.sh` is embedded in the launcher binary rather than fetched from
metadata so its contract evolves in lockstep with the launcher code.

## Archive handling

The disk asset published in S3 may be a raw `.img` file or an archived bundle.
When `payload.disk.filename` ends in `.tar.xz`, the launcher decompresses and
untars the wire artifact after sha256 verification and selects the first
`*.img` entry inside as the cached disk source. The decompressed image lives
alongside the verified archive in the same per-version cache directory so a
subsequent deploy reuses the extracted result without re-decompressing.

The `tar -xJf` shell-out is used (avoids adding a Go xz dependency; both
macOS and Linux ship `tar` and `xz` in the base toolchain). Other filename
suffixes are passed through to the disk-staging step as-is.

The `.run` asset is not subject to extraction; it is delivered to the guest
as-is at `/mnt/host/db.run` and the guest runs it through `start.sh`.

## Disk staging

The downloaded disk image is cached in the user-cache directory (shared across
deployments). A writable disk image cannot be mounted into multiple VMs
simultaneously without corruption, so each deployment receives its own copy.

On first deploy:

1. Resolve the cached source path from `PayloadRef.DiskImagePath`.
2. Copy it to `<deploymentDir>/local-runtime/vm/disk.img`. Prefer APFS reflink
   (`cp -c`) on macOS for an effectively free copy; fall back to a stream copy.
3. Persist the per-deployment disk path; reuse on subsequent deploys.

On subsequent deploys:

- If the per-deployment disk already exists and the recorded payload identity
  matches the current selection, reuse it.
- If the cached source identity differs (different version selected), restage.

The EFI variable store at `<deploymentDir>/local-runtime/vm/efi-vars.fd` is
created by the EFI bootloader on first boot and persisted thereafter.

## Stop pipeline (the only meaningful behavioral addition)

The current stop pipeline relies on a guest-side daemon listening on a virtio-fs
control socket. The EFI-booted Alpine guest does not provide this daemon, so the
existing `Controller.RequestGracefulStop` socket dial fails, and its fallback is
a host-side file write at `Controller.Paths().HostStopRequestPath`. Today
nothing reads that file.

Add a watcher in the runner:

- After `driver.Start` succeeds, the runner spawns a goroutine that polls
  `Controller.Paths().HostStopRequestPath` at a fixed interval (e.g.,
  `controlPollInterval = 100ms` already defined in `controller.go`).
- When the stop-request file appears, the goroutine cancels a context that
  triggers `driver.Stop(ctx)`. `driver.Stop` already calls `vz.RequestStop`
  (ACPI), which Alpine handles cleanly via its OpenRC shutdown.
- VZ transitions to Stopped, `driver.Wait` returns, the runner process exits,
  and the launcher's existing `WaitForRunnerExit` succeeds.

The graceful-stop socket path stays in the codebase (it is harmless dead code
for EFI-Alpine and can be removed in a later cleanup change).

## Wait-for-ready (no change needed)

`localBackend.Deploy` already polls the database via `verifyDatabaseConnectionFn`
in `waitForLocalRuntimeStarted`. The polling probes the connection through the
loopback forwarder. Once `start.sh` (run by `exasol-bootstrap`) has invoked
`db.run` and the DB binds `:8563`, the forwarder accepts the host-side
connection, the probe succeeds, and the deployment is marked running.

## Connection information (no change needed)

`writeLocalArtifacts` already populates `deployment.json` with
`Host: "127.0.0.1"`, the host-allocated `DBPort` and `UIPort`, the launcher-owned
default SQL credentials, and `ShellSupported: false`. `exasol info` and
`exasol connect` consume that schema unchanged.

## Asset metadata

### Schema before

```json
{
  "payloads": [{
    "version": "...",
    "architecture": "arm64",
    "url": "...",         "sha256": "...", "filename": "...",
    "boot": {
      "kernel": { "url": "...", "sha256": "...", "filename": "..." },
      "initrd": { "url": "...", "sha256": "...", "filename": "..." }
    }
  }]
}
```

### Schema after

```json
{
  "payloads": [{
    "version": "...",
    "architecture": "arm64",
    "disk": { "url": "...", "sha256": "...", "filename": "..." },
    "run":  { "url": "...", "sha256": "...", "filename": "..." }
  }]
}
```

The top-level `url`/`sha256`/`filename` fields are removed. Two new `Asset`
sub-structures carry the EFI disk image and the Exasol installer binary
respectively. The `boot` substructure is removed.

This is structurally clearer than repurposing the top-level fields and aligns
with the existing `Asset` type used by `BootAssets.Kernel` and `BootAssets.Initrd`
in the previous schema.

## Backward compatibility

None. Old metadata describing kernel + initrd is rejected. Old metadata
describing a primary `.run` URL at the top level is also rejected.

The kernel + initrd path remains available on the
`dm.openspec_added_local_install` branch if it ever needs to be referenced.

## Risks

- The vz/v3 binding may not export an EFI bootloader constructor in the
  currently vendored version. The pre-flight task in `tasks.md` requires the
  agent to verify and bump if needed before any code changes.
- The hardcoded guest IP `192.168.64.2` in `runner.go` assumes VZ NAT's
  first-guest convention. With an EFI-booted Alpine guest using a randomly
  assigned MAC, this should hold for single-deployment scenarios; if it fails
  in practice, it is the same defect class that exists for the kernel-boot
  path today and is fixed in a separate change (likely IP discovery).
- Concurrent deployments must not share the per-deployment disk file; the
  disk-staging step is the safety boundary.
- The Alpine image must include the `exasol-bootstrap` OpenRC service and the
  `xz` and `bash` packages required by the `.run` self-extractor. If the image
  is built incorrectly, `start.sh` will fail and the deploy will time out.
  This is an `exasol-nano-vm` correctness boundary, not a launcher defect.
- The shared dir is read-write so the guest can write back log output or status
  files. A future tightening could split this into a read-only payload share
  and a separate read-write logs share.
