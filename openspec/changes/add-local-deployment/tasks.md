# Tasks: Add Local Deployment Mode

## Phase 1: Lifecycle and preset foundations

- [x] Add backend metadata to infrastructure manifests and a backend resolver in `internal/deploy`.
- [x] Move current cloud lifecycle behavior behind a `tofuBackend`.
- [x] Add compatibility metadata to infrastructure and installation manifests.
- [x] Validate preset compatibility during `init` and `install` before deployment-directory mutation.
- [x] Extend install-step execution so `localCommand` tasks are passed through alongside `remoteExec`.

## Phase 2: Local runtime foundations

- [x] Add `internal/localruntime` package scaffolding with deployment-scoped runtime root handling.
- [x] Add local runtime state model and persistence under `<deploymentDir>/local-runtime/state.json`.
- [x] Add local port allocation and persisted reuse.
- [x] Add payload metadata parsing, HTTP download, checksum verification, and cache management.

## Phase 3: macOS arm64 runtime integration

- [x] Add `internal/localruntime/vm` abstraction with unsupported stubs for non-darwin or non-arm64 builds.
- [x] Implement darwin/arm64 VM driver using `Virtualization.framework` directly or through a thin wrapper.
- [x] Implement the mounted control socket/file bridge between host and guest.
- [x] Add guest bootstrap logic that boots the local VM and invokes the Linux ExaNano `.run` payload inside the guest under local runtime control.

## Phase 4: Local backend and user-facing behavior

- [x] Implement `localBackend` for `deploy`, `start`, `stop`, and `destroy`.
- [x] Add `local` infrastructure preset and dedicated local installation preset such as `nano`.
- [x] Make `exasol install local` resolve only to compatible installation presets.
- [x] Generate local-safe `deployment.json`, `secrets.json`, and `connection-instructions.txt`.
- [x] Make `info`, `connect`, `status`, and `diag info` local-aware.
- [x] Make `shell host`, `shell container`, and `diag shell` fail with explicit local-unsupported messages.

## Phase 5: Build and release

- [x] Split macOS arm64 launcher build settings from the generic build matrix.
- [x] Add signing and notarization support required for the virtualization-enabled launcher.
- [x] Publish Linux ExaNano `.run` payloads and any required guest boot assets as versioned artifacts to a product-owned HTTP location.
- [x] Update installer and CI/release flows to support the `.run`-based local deployment payload contract on macOS arm64.

## Validation

- [x] Add unit tests for backend resolution and preset compatibility validation.
- [x] Add unit tests for local runtime state, payload verification, and port persistence.
- [x] Add integration tests for local lifecycle commands without cloud credentials.
- [x] Add concurrent local deployment tests using two deployment directories.
- [x] Add macOS arm64 platform validation for build, runtime, and installer behavior with the `.run`-based guest payload flow.
