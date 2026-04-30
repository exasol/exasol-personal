# Proposal: Replace Kernel + Initrd Boot With EFI Disk Boot

## Why

The local runtime currently boots a custom kernel + initrd contract
(`vmlinux.container` + `ubuntu-initrd.cpio.gz`) via Apple Virtualization.framework's
Linux bootloader. The launcher controls the guest's init via custom `exa_*` kernel
command-line parameters and a constellation of virtio-fs shares.

The platform direction is to consume a generic Alpine VM image produced by
`exasol-nano-vm` and boot it via EFI. The shipped Alpine image is intentionally
generic: it boots, mounts a single virtio-fs share at `/mnt/host`, and runs an
OpenRC `exasol-bootstrap` service that executes `/mnt/host/start.sh` on every
boot if the file is present. The image has no Exasol-specific knowledge and
ships no DB binary.

The launcher remains the owner of the guest execution contract but delivers it
through the share rather than through kernel command-line parameters and a
custom initrd. Specifically, before booting the VM, the launcher stages
`db.run` (the Exasol installer payload) and `start.sh` (the launcher's
host-authored boot hook) into a per-deployment payload-share directory that is
mounted at `/mnt/host` inside the guest. The OpenRC `exasol-bootstrap` service
inside the image picks them up and runs them.

This change replaces the kernel + initrd boot path with EFI disk boot end to end,
so that `exasol install local`, `exasol deploy`, `exasol connect`, `exasol stop`,
and `exasol destroy` all work against the EFI-booted Alpine image with the
launcher delivering the DB payload via virtio-fs.

It is a prototype-stage replacement: there are no production users on the
kernel + initrd path, so no compatibility shim is needed.

## What Changes

- Replace `vz.NewLinuxBootLoader` with the vz/v3 EFI bootloader binding in the
  darwin/arm64 driver.
- Replace the kernel + initrd payload metadata schema with a payload entry
  containing two assets: an EFI `disk` image (the generic Alpine VM) and a
  `run` binary (the Exasol installer the launcher delivers to the guest).
- Remove the bundle-extraction layer (`internal/localruntime/assets/bundle.go`).
- Remove the kernel-boot-specific guest-bootstrap scaffolding: the four old
  virtio-fs shares (`exa-payload`, `exa-provision`, `exa-logs`, `exa-control`),
  embedded `profile.sh` and entrypoint wrapper, layer-disk image creation,
  kernel-command-line builder, and bootstrap layer key.
- Replace `PrepareGuest` so it produces a `MachineConfig` for EFI boot with: a
  per-deployment EFI disk path, a per-deployment EFI variable store path, and a
  single read-write virtio-fs share (`hostshare` → `/mnt/host`) backed by the
  per-deployment payload-share directory.
- Add per-deployment disk staging: copy the cached disk image into the
  deployment directory before first boot so concurrent deployments do not share
  a writable disk; reuse on subsequent boots.
- Add per-deployment payload-share staging: copy the cached `db.run` into the
  payload-share directory and write a launcher-authored `start.sh` alongside it.
  The guest's `exasol-bootstrap` OpenRC service executes `start.sh` on boot.
- Add a stop-request watcher in the local runtime runner so a
  `Controller.RequestStop` file write triggers `driver.Stop` (VZ ACPI shutdown)
  and the VM exits cleanly. This replaces the guest-side control-socket
  cooperation that the EFI-booted Alpine guest does not provide.
- Tighten `ValidateEnvironment` to require macOS 13 (Ventura) — the minimum that
  supports `VZEFIBootLoader`.

## Impact

This change delivers a working `exasol install local` → `exasol connect`
end-to-end against an EFI-booted generic Alpine image fed by a launcher-staged
payload share. Specifically, after this change:

- `exasol install local` boots the VM, the guest's `exasol-bootstrap` service
  runs the launcher-authored `start.sh` which executes `db.run`, and the
  launcher waits for the database to respond on the forwarded loopback port.
- `exasol connect` opens a SQL session via the existing
  `LoopbackForwarder` chain (`127.0.0.1:<allocated>` → `192.168.64.2:8563`).
- `exasol stop` triggers a clean ACPI shutdown of the Alpine guest.
- `exasol destroy` removes the deployment directory (including the staged
  payload share) and runtime root.

Out of scope:

- Discovering the guest IP dynamically — the existing hardcoded
  `defaultGuestIPv4 = "192.168.64.2"` in `runner.go` is preserved.
- A guest-to-host control channel for graceful DB shutdown handshakes — stop
  remains "host writes stop-request file → host watcher requests ACPI shutdown
  → guest OpenRC sequence shuts the DB down via SIGTERM."
- Restart-on-crash supervision inside the guest — if the DB crashes, it stays
  down until the next VM boot. A future change can add `supervise-daemon` or
  equivalent inside `start.sh`.

Impacted capability areas:

- local runtime payload distribution and caching
- local deployment lifecycle (boot path, payload-share staging, stop pipeline)
- local runtime guest preparation

Impacted code areas:

- `internal/localruntime/assets/`
- `internal/localruntime/payload_selection.go`
- `internal/localruntime/state/` (`PayloadRef` shape)
- `internal/localruntime/config/layout.go` (payload-share path)
- `internal/localruntime/vm/driver_darwin_arm64.go`
- `internal/localruntime/vm/driver.go` (`MachineConfig` struct)
- `internal/localruntime/guest.go` and `guest_assets.go`
- `internal/localruntime/disk_staging.go`, plus a new payload-share staging
  helper and embedded `start.sh`
- `internal/localruntime/runner.go` (stop-request watcher)
- `internal/deploy/local_platform.go` (macOS Ventura check)
- Tests under all affected packages
