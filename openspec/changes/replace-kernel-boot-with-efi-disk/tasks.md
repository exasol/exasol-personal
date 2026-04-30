# Tasks: Replace Kernel + Initrd Boot With EFI Disk Boot

## Phase 0: Pre-flight

- [x] Confirm `github.com/Code-Hex/vz/v3` exposes an EFI bootloader constructor
      (e.g. `NewEFIBootLoader`) in the vendored version. Run
      `grep -rn "EFIBootLoader" $(go env GOMODCACHE)/github.com/!code-!hex/vz/v3*/`.
      If empty, run `go get github.com/Code-Hex/vz/v3@latest && go mod tidy` and
      commit `go.mod` / `go.sum` as a standalone commit before any code edits.
      If the latest version still does not expose an EFI loader, pause and
      report — do not invent an API.
- [x] Read these files in full before making changes; the spec assumes their
      current shape and the agent must adjust if behavior diverges:
  - `internal/localruntime/runner.go`
  - `internal/localruntime/runtime.go`
  - `internal/localruntime/controller.go`
  - `internal/localruntime/vm/driver.go`
  - `internal/localruntime/vm/driver_darwin_arm64.go`
  - `internal/localruntime/vm/driver_unsupported.go`
  - `internal/localruntime/vm/config_helpers.go`
  - `internal/deploy/local_backend.go`
  - `internal/deploy/local_platform.go`

## Phase 1: Asset metadata schema

- [x] In `internal/localruntime/assets/manager.go`:
  - Remove the `BootAssets` struct.
  - Remove `Boot *BootAssets` from `Payload`.
  - Remove `EnsureBootCached` and `CachedBootAssets`.
  - Remove `Payload.URL`, `Payload.SHA256`, `Payload.Filename` fields.
  - Add `Disk *Asset` to `Payload`.
  - Update `EnsureCached` (currently caches the primary `Payload.URL`/etc.) to
    cache `payload.Disk` instead. Returned cache path is the disk image path.
  - Keep `Metadata`, `Asset`, `NewManager`, `DefaultCacheDir`, `Resolve`,
    `fetchMetadata`, `verifyFileSHA256`, the
    `EXASOL_LOCAL_RUNTIME_PAYLOAD_METADATA_URL` env var, and the
    `DefaultPayloadMetadataURL` constant.
  - The cache layout `<userCacheDir>/exasol-personal/localruntime/payloads/<version>/<arch>/`
    is preserved; the asset's resolved filename is used as the leaf, no `boot/`
    subdirectory.
- [x] Delete `internal/localruntime/assets/bundle.go`.
- [x] Delete `internal/localruntime/assets/bundle_test.go`.
- [x] Confirm no surviving Go imports of the deleted bundle types
      (`Bundle`, `PrepareBundle`, `ErrPayloadBundleInvalid`).

## Phase 2: Payload selection and state

- [x] In `internal/localruntime/state/`:
  - Remove the `Boot *PayloadBootRef` field on `PayloadRef`.
  - Remove the `PayloadBootRef` struct.
  - Add a `DiskImagePath string` field to `PayloadRef` (the cached disk image
    path).
- [x] In `internal/localruntime/payload_selection.go`:
  - Remove `ErrPayloadBootAssetsMissing`.
  - In `EnsurePayloadSelected`, replace the boot-required validation with a
    check that `payload.Disk != nil && payload.Disk.URL != "" && payload.Disk.SHA256 != ""`.
  - Replace the `manager.EnsureBootCached` call with a single
    `manager.EnsureCached(ctx, payload)` call (which now caches the disk).
  - Remove the assignment to `state.Payload.Boot`; assign the cached path to
    `state.Payload.DiskImagePath` instead.
  - Update `cachedPayloadRef` to validate that `state.Payload.DiskImagePath` is
    a present, non-directory file (use the existing `isCachedFile` helper).
  - Remove `EnsureBootCached` from any local `payloadManager` interface.

## Phase 3: VM driver — EFI bootloader

- [x] Confirm where `MachineConfig` is defined (likely `internal/localruntime/vm/driver.go`)
      using `grep -n "type MachineConfig" internal/localruntime/vm/`.
- [x] In that file:
  - Remove `KernelPath`, `InitrdPath`, `KernelCommandLine` fields.
  - Add `DiskImagePath string` (required for boot).
  - Add `EFIVarsPath string` (per-deployment EFI variable store path).
  - Keep `Name`, `CPUCount`, `MemoryBytes`, `MachineIdentifierPath`,
    `ConsoleLogPath`, `SharedDirs`, `PortForwards`. If a separate `DiskImage`
    field exists, fold it into `DiskImagePath` (one canonical name).
- [x] In `internal/localruntime/vm/driver_darwin_arm64.go`:
  - Replace the body of `buildBootLoader` to call the vz/v3 EFI bootloader
    constructor configured against `config.EFIVarsPath`. The signature of the
    helper changes from returning `*vz.LinuxBootLoader` to whatever the EFI
    binding returns; update its return type accordingly. Adjust the call site
    in `buildVirtualMachineConfiguration`.
  - In `validateMachineConfig`:
    - Remove the `KernelPath required` check.
    - Add a `DiskImagePath required` check.
    - Add an `EFIVarsPath required` check.
    - Keep the existing `Name required`, port-forward rejection, and shared-dir
      tag checks unchanged.
  - Ensure `buildStorageDevices(config.DiskImagePath)` continues to attach the
    disk read-write — the existing call
    `vz.NewDiskImageStorageDeviceAttachment(path, false)` keeps the disk
    writable (`false` = not read-only) and that is correct for EFI boot.
- [x] In `internal/localruntime/vm/driver_unsupported.go`:
  - Update any signature changes from the darwin/arm64 driver. Behavior
    unchanged: returns an unsupported-platform error.
- [x] In `internal/localruntime/vm/config_helpers.go`:
  - Remove helpers that exclusively serve kernel + initrd config, if any.
  - Keep CPU/memory clamping helpers and shared-dir tag resolution.

## Phase 4: Per-deployment disk staging

- [x] Add a method to the `Runtime` (likely in `runtime.go` or a new file in
      `internal/localruntime/`):
  - `StagedDiskImagePath() string` returns
    `<deploymentDir>/local-runtime/vm/disk.img`.
  - `EFIVarsPath() string` returns
    `<deploymentDir>/local-runtime/vm/efi-vars.fd`.
- [x] Add a `StageDiskImage` helper that:
  - Ensures the parent directory exists with mode `localRuntimeDirMode`.
  - If the staged disk does not exist, copies the cached source to the staged
    path. Prefer APFS reflink via
    `exec.Command("cp", "-c", source, target)`; on non-zero exit, fall back to
    a stream copy.
  - Records the source identity (cached path + checksum) alongside the staged
    disk so a subsequent call with a different source triggers a restage.
  - Returns the staged path.
  - Idempotent: on second call with same source identity, becomes a no-op.

## Phase 5: Guest preparation rewrite

- [x] In `internal/localruntime/guest.go`:
  - Replace `PrepareGuest` with a minimal implementation:
    - Validate context (`ctx.Err()`).
    - Call `r.EnsureRoot()`.
    - Call `r.LoadState()`.
    - Require `state.Payload != nil` and `state.Payload.DiskImagePath != ""`
      (error `ErrPayloadSelectionMissing` otherwise).
    - Load machine sizing via `r.LoadMachineSizing()`.
    - Call `r.StageDiskImage(state.Payload.DiskImagePath)` and use the staged
      path.
    - Build `MachineConfig` with: `Name` (use existing `deploymentMachineName`
      if it is kept; otherwise inline), `CPUCount`, `MemoryBytes`,
      `DiskImagePath` (staged), `EFIVarsPath`, `MachineIdentifierPath`,
      `ConsoleLogPath`. Leave `SharedDirs` as nil (no virtio-fs shares).
    - Return `&GuestConfig{Controller: r.Controller(), Machine: machineConfig}`.
      `Controller.Ensure()` is **not** called from `PrepareGuest` in this
      change — the runner calls it (Phase 6) so the stop-request path exists.
  - Delete the following helpers (no callers remain after this change):
    - `resolveBootAssets`
    - `bootAssets` type
    - `preparePayloadShare`
    - `resolvePayloadExecutablePath`
    - `stagedPayloadRefreshRequired`
    - `stagePayloadExecutable`
    - `prepareBootstrapShare`
    - `buildKernelCommandLine`
    - `bootstrapLayerKey`
    - `payloadValue`
    - `ensureLayerDiskImage`
    - `writePayloadChecksum` (if not used by the new staging helper)
    - `copyFile` (keep if reused by the staging helper's stream-copy fallback;
      otherwise delete)
    - `deploymentMachineName` (keep if used by new `PrepareGuest`)
  - Delete the constants no longer referenced:
    - `defaultKernelAppend`
    - `defaultRestartPolicy`
    - `defaultGuestProvisionTag`, `defaultGuestProvisionMount`
    - `defaultGuestPayloadTag`, `defaultGuestPayloadMount`
    - `defaultGuestLogsTag`, `defaultGuestLogsMount`
    - `entrypointWrapperFileName`
    - `bootstrapProfileFileName`
  - Keep `localRuntimeDirMode`, `localRuntimeFileMode`, `localRuntimeExecFileMode`
    if used by Phase 4 or surviving code; otherwise relocate.
- [x] Delete `internal/localruntime/guest_assets.go`.
- [x] Delete `internal/localruntime/guest/profile.sh`.
- [x] Delete `internal/localruntime/guest/exasol-localruntime-entrypoint.sh`.
- [x] If `internal/localruntime/guest/` becomes empty, delete the directory.

## Phase 6: Runner stop-request watcher

- [x] In `internal/localruntime/runner.go`:
  - In `Run`, after `driver.Start` succeeds:
    - Ensure the controller's host-side state exists by calling
      `guest.Controller.Ensure()` (creates `ControlDir`, clears any stale
      stop-request file).
    - Spawn a goroutine that polls `guest.Controller.Paths().HostStopRequestPath`
      with `time.Tick(controlPollInterval)`. When the file is detected, call
      `driver.Stop(<derived ctx>)` and exit the goroutine.
    - Use a derived context (`stopCtx, stopCancel := context.WithCancel(ctx)`)
      so the watcher exits cleanly when `Run` returns. Defer `stopCancel()`.
  - Keep the existing `driver.Wait(ctx)` call. When `driver.Stop` succeeds, the
    VM transitions to stopped and `driver.Wait` returns nil; the runner exits.
  - Keep the existing forwarder lifecycle (`StartLoopbackForwarder` for SQL
    and UI ports targeting `defaultGuestIPv4`). No changes required here.
- [x] No changes needed in `internal/deploy/local_backend.go::Stop` — it
      already calls `Controller.RequestGracefulStop`, whose socket path will
      fail (no guest daemon) and fall through to the file-based `RequestStop`
      that this watcher now consumes. The existing `WaitForRunnerExit` call
      then succeeds when the runner exits.

## Phase 7: Environment validation

- [x] In `internal/deploy/local_platform.go` (read first to understand current
      check shape):
  - Tighten the existing macOS arm64 check to also require macOS 13 (Ventura)
    or later.
  - Use `golang.org/x/sys/unix` `Sysctl("kern.osproductversion")` or
    `unix.Uname` parsing — pick whichever the rest of the codebase uses; if
    neither is available, use `runtime.GOOS == "darwin"` plus a sysctl call via
    `os/exec` of `sw_vers -productVersion`. Compare lexicographically by major
    version (≥ 13).
  - Update the error message to read: "local deployment requires Apple Silicon
    macOS 13 (Ventura) or later because EFI VM boot is unavailable on earlier
    versions".

## Phase 8: Tests

- [x] In `internal/localruntime/assets/manager_test.go`:
  - Replace metadata fixtures with the new disk-shaped schema
    (`payloads[].disk.{url,sha256,filename}`).
  - Remove tests asserting `BootAssets`, `EnsureBootCached`, or
    `CachedBootAssets`.
  - Add tests:
    - `Resolve` returns a payload by architecture from disk-shaped metadata.
    - `EnsureCached` writes the disk to the expected
      `<version>/<arch>/<filename>` cache path.
    - SHA256 mismatch is rejected with `ErrPayloadVerificationFailed`.
    - Cached file is reused on second call without re-download (use a custom
      `http.Client` round-tripper that fails the second request).
- [x] Delete `internal/localruntime/assets/bundle_test.go` (already noted in
      Phase 1).
- [x] In `internal/localruntime/payload_selection_test.go`:
  - Remove tests asserting boot-asset presence or absence behavior.
  - Add tests:
    - `EnsurePayloadSelected` succeeds with disk-only payload metadata and
      records `DiskImagePath` in state.
    - `EnsurePayloadSelected` errors when `payload.Disk` is nil or
      missing URL / SHA256.
    - `cachedPayloadRef` returns nil when the recorded `DiskImagePath` is no
      longer present on disk.
- [x] In `internal/localruntime/guest_test.go`:
  - Remove `TestRuntimePrepareGuest_FailsWithoutBootAssets`.
  - Update `TestRuntimePrepareGuest_BuildsMachineConfigFromSelectedRunPayload`
    (rename to `..._FromSelectedDiskImage`):
    - Fixture state has `Payload.DiskImagePath` set to a temp file.
    - Assert resulting `MachineConfig.DiskImagePath` is the staged path under
      `<deploymentDir>/local-runtime/vm/disk.img`.
    - Assert `MachineConfig.EFIVarsPath` is the deployment EFI vars path.
    - Assert `MachineConfig.SharedDirs` is empty.
  - Adjust `TestRuntimePrepareGuest_FailsWithoutSelectedPayload` to omit
    `DiskImagePath` from state.
- [x] In `internal/localruntime/vm/driver_test.go`:
  - Remove tests asserting kernel-path or initrd-path validation.
  - Add tests:
    - `validateMachineConfig` rejects empty `DiskImagePath`.
    - `validateMachineConfig` rejects empty `EFIVarsPath`.
- [x] Add `internal/localruntime/disk_staging_test.go` (or an equivalent
      location near the new `StageDiskImage` helper):
  - First call copies cached disk to per-deployment path.
  - Second call with same source is a no-op (file mtime unchanged).
  - Different source triggers a restage.
  - Stream-copy fallback works on a non-APFS-style temp dir (mock the
    `exec.Command("cp", "-c", ...)` failure).
- [x] Add a runner-level test for the stop-request watcher:
  - Mock the VZ driver (`newVMDriver` is overridable via a package var).
  - Start the runner with a stub driver whose `Wait` blocks until `Stop` is
    called.
  - Write the stop-request file. Assert `Stop` is invoked on the driver
    within a small timeout.
  - Assert `Run` returns nil after stop.
- [x] Run `task tests-unit`. Adjust call sites surfaced by compile errors.

## Phase 9: Build verification

- [x] `task lint-golang` clean.
- [x] `task tests-unit` clean.
- [x] `task build` succeeds on Linux (driver_unsupported applies).
- [ ] On a darwin/arm64 host (CI or developer), `task build` succeeds with the
      new EFI-only driver.

## Phase 12: Payload share and `.run` delivery

This phase adds back launcher→guest payload delivery via a single virtio-fs
share. The disk image is now generic Alpine + an `exasol-bootstrap` OpenRC
service; the launcher delivers `db.run` and a launcher-authored `start.sh`
into a per-deployment payload-share directory mounted at `/mnt/host` in the
guest.

### 12.1: Asset metadata schema (assets package)

- [x] In `internal/localruntime/assets/manager.go`:
  - Add `Run *Asset` to `Payload` (alongside the existing `Disk *Asset`).
  - Update `EnsureCached` to cache both `payload.Disk` and `payload.Run`.
    Decide whether to return them as separate values (e.g., return type
    `CachedPayload struct{ DiskPath, RunPath string }`) or to expose two
    methods (`EnsureDiskCached`, `EnsureRunCached`). Pick whichever keeps
    the payload-selection call site simpler.
  - Cache layout `<userCacheDir>/exasol-personal/localruntime/payloads/<version>/<arch>/<filename>`
    is preserved; both assets are leaf-named by their resolved filename.

### 12.2: Payload selection and state

- [x] In `internal/localruntime/state/store.go`:
  - Add `RunPath string` to `PayloadRef` alongside `DiskImagePath`.
- [x] In `internal/localruntime/payload_selection.go`:
  - Validate `payload.Disk != nil` AND `payload.Run != nil`, with non-empty
    URL/SHA256 on each. Use a single error sentinel
    (`ErrPayloadAssetMissing` or rename `ErrPayloadDiskAssetMissing`) that
    mentions which asset is missing.
  - Cache both, persist both paths in `state.Payload`.
  - Update `cachedPayloadRef` to require BOTH `DiskImagePath` and `RunPath`
    to refer to present, non-directory files.

### 12.3: Layout

- [x] In `internal/localruntime/config/layout.go`:
  - Add `PayloadShareDir() string` returning
    `<deploymentDir>/local-runtime/vm/payload-share/`.
- [x] In `internal/localruntime/runtime.go::EnsureRoot`:
  - Add `PayloadShareDir()` to the list of dirs created.

### 12.4: Embedded start script

- [x] Add `internal/localruntime/start.sh` (POSIX shell) and embed it via
      `//go:embed`. Approximate behavior:
  ```sh
  #!/bin/sh
  set -e
  export HOME="${HOME:-/root}"
  RUN=/mnt/host/db.run
  if [ ! -x "$RUN" ]; then
    echo "start.sh: $RUN not found or not executable" >&2
    exit 1
  fi
  # detach so OpenRC service exits while DB keeps running
  nohup "$RUN" >> /var/log/exasol-db.log 2>&1 &
  echo $! > /var/run/exasol-db.pid
  ```
  Exact contents are the launcher's runtime contract; iterate as needed.

### 12.5: Payload-share staging helper

- [x] Add a method to `Runtime` (in `disk_staging.go` or a new file
      `payload_share_staging.go`):
  - `StagePayloadShare(cachedRunPath string) error` that:
    - Ensures `PayloadShareDir()` exists with mode `localRuntimeDirMode`.
    - Writes the embedded `start.sh` into `<PayloadShareDir>/start.sh` with
      mode `localRuntimeExecFileMode` (0o700) on every call (rewrites
      unconditionally so launcher-version updates take effect).
    - Stages the cached `.run` into `<PayloadShareDir>/db.run` with mode
      `localRuntimeExecFileMode`. Use checksum-based skip-on-unchanged via
      a sidecar `<PayloadShareDir>/.db.run.sha256`. Prefer hardlink/reflink;
      fall back to stream copy.
- [x] Reintroduce or re-use `localRuntimeExecFileMode = 0o700` constant.

### 12.6: PrepareGuest

- [x] In `internal/localruntime/guest.go::PrepareGuest`:
  - After `StageDiskImage`, call `StagePayloadShare(state.Payload.RunPath)`.
  - Set `MachineConfig.SharedDirs` to a single entry:
    ```go
    {Tag: "hostshare", Source: r.layout.PayloadShareDir(),
     Destination: "/mnt/host", ReadOnly: false}
    ```
  - Validate `state.Payload.RunPath != ""` alongside `DiskImagePath`.

### 12.7: VM driver

- [x] No driver changes expected — `MachineConfig.SharedDirs` and
      `buildDirectoryShares` already handle a single share. Verify:
  - `validateMachineConfig` continues to accept one shared dir with tag
    `hostshare`.
  - `buildDirectoryShares` produces a virtio-fs configuration consumed by VZ.

### 12.8: Tests

- [x] In `internal/localruntime/assets/manager_test.go`:
  - Update metadata fixtures to include both `disk` and `run`.
  - Add a test asserting both are cached, with the expected leaf paths.
  - Add a test asserting payload without `run` fails validation.
- [x] In `internal/localruntime/payload_selection_test.go`:
  - Update fixtures to include both `disk` and `run`.
  - Assert `state.Payload.RunPath` is populated.
  - Assert `cachedPayloadRef` returns nil when `RunPath` file is absent.
- [x] In `internal/localruntime/guest_test.go`:
  - Update `TestRuntimePrepareGuest_BuildsMachineConfigFromSelectedDiskImage`:
    - Fixture state has both `DiskImagePath` and `RunPath`.
    - Assert `MachineConfig.SharedDirs` has exactly one entry with tag
      `hostshare`, source under `<deploymentDir>/local-runtime/vm/payload-share/`,
      destination `/mnt/host`.
    - Assert `<PayloadShareDir>/db.run` and `<PayloadShareDir>/start.sh`
      exist and are executable.
- [x] Add `internal/localruntime/payload_share_staging_test.go`:
  - First call writes both files; both are executable.
  - Same `.run` content on second call → no-op for `db.run` (mtime
    unchanged).
  - `start.sh` is rewritten on every call (mtime updates).
  - Different `.run` content → restage.

### 12.10: Disk archive extraction

The exasol-nano-vm packaging produces `mac-arm64.tar.xz`. The launcher should
extract `.tar.xz` archives after sha256 verification.

- [x] In `internal/localruntime/assets/manager.go` (or a sibling helper file
      like `archive.go`):
  - After the disk asset's sha256 verifies and is renamed into the cache,
    check `asset.resolvedFilename()` for a `.tar.xz` suffix.
  - If yes, extract via `exec.Command("tar", "-xJf", archivePath, "-C",
    extractDir)` into a sibling cache subdirectory (e.g.,
    `<cache>/<version>/<arch>/extracted/`). Walk the extract dir, pick the
    first `*.img` file, and return that path from `EnsureCached` for the
    disk asset.
  - If the extracted dir already contains a usable `.img` and the wire
    artifact's sha256 matches, reuse it (no re-extraction).
  - The `.run` asset is unchanged — passed through as-is.
- [x] Add tests in `internal/localruntime/assets/manager_test.go`:
  - Build a small `.tar.xz` fixture in a temp dir containing a fake
    `bundle/disk-fixture.img` plus a sibling file. Assert `EnsureCached`
    returns the path to the inner `.img`.
  - Assert second call reuses the extraction without re-running tar (use a
    custom `http.Client` that fails the second metadata fetch — actually
    `EnsureCached` doesn't refetch metadata; just confirm via mtime that
    the inner img is unchanged).
  - Assert non-`.tar.xz` filenames pass through (keep existing test).
- [x] Document the supported archive formats in design.md (already done in
      the amendment that added this task).

### 12.9: Phase 9 build verification (re-run)

- [x] `task lint-golang` clean.
- [x] `task tests-unit` clean.
- [x] `task build` succeeds on Linux (driver_unsupported applies).
- [ ] On a darwin/arm64 host, `task build` succeeds.

## Phase 10: Manual smoke test (developer-only, not CI)

Performed on a Mac (macOS 13+, Apple Silicon). Requires:
- A generic Alpine EFI image built by `exasol-nano-vm` (Flavor B: image runs
  `/mnt/host/start.sh` on boot via the `exasol-bootstrap` OpenRC service; no
  DB-specific knowledge baked in).
- The Exasol `.run` binary for the host architecture.

- [ ] Place the disk image at `~/tmp/exasol-nano-vm.img` and the installer
      at `~/tmp/exasol-nano-db.run`. Compute both checksums:
      `shasum -a 256 ~/tmp/exasol-nano-vm.img ~/tmp/exasol-nano-db.run`.
- [ ] Create `~/tmp/metadata.json`:
    ```json
    {
      "payloads": [{
        "version": "smoke-test-0",
        "architecture": "arm64",
        "disk": {
          "url": "file:///Users/<you>/tmp/exasol-nano-vm.img",
          "sha256": "<computed>",
          "filename": "exasol-nano-vm.img"
        },
        "run": {
          "url": "file:///Users/<you>/tmp/exasol-nano-db.run",
          "sha256": "<computed>",
          "filename": "exasol-nano-db.run"
        }
      }]
    }
    ```
- [ ] `export EXASOL_LOCAL_RUNTIME_PAYLOAD_METADATA_URL=file:///Users/<you>/tmp/metadata.json`
- [ ] `exasol install local` against a fresh deployment dir.
- [ ] Verify each of:
  - `<deploymentDir>/local-runtime/vm/disk.img` exists (staged copy).
  - `<deploymentDir>/local-runtime/vm/efi-vars.fd` is created on first boot.
  - `<deploymentDir>/local-runtime/vm/payload-share/db.run` exists, is
    executable, and matches the source checksum.
  - `<deploymentDir>/local-runtime/vm/payload-share/start.sh` exists, is
    executable, and matches the launcher's embedded copy.
  - Console log shows EFI firmware boot, kernel boot, and the
    `exasol-bootstrap` service running `start.sh`.
  - The runner process is alive (`ps`).
  - The forwarders are listening (`lsof -i :<dbPort>` and `:<uiPort>`).
  - `verifyDatabaseConnectionFn` succeeds within the wait timeout (default
    `StartedDefaultTimeoutSeconds`).
  - `exasol info` reports `running`.
  - `exasol connect` opens a SQL session with `sys` / `exasol`.
  - `exasol stop` shuts the VM down within `localStopTimeout` (60 s) and
    returns cleanly.
  - `exasol destroy` removes the deployment directory artifacts including
    `payload-share/`.
- [ ] Verify the second deploy reuses the staged disk image and the staged
      `db.run` (mtime unchanged for both), and a metadata change with a
      different `disk.sha256` or `run.sha256` triggers the corresponding
      restage.

## Phase 11: Spec deltas

- [x] Confirm the spec deltas at
      `openspec/changes/replace-kernel-boot-with-efi-disk/specs/local-runtime-assets/spec.md`
      and
      `openspec/changes/replace-kernel-boot-with-efi-disk/specs/local-deployment/spec.md`
      match the implementation.
- [x] Run `openspec validate --change replace-kernel-boot-with-efi-disk`. If it
      complains about the modify-intent heading format
      (`## REMOVED`, `## MODIFIED`, `## ADDED`), normalize the headings to the
      schema's expected delta format and re-run.
