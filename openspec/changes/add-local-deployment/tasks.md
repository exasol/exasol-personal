# Tasks: Add Local Deployment Mode

## Phase 1: Lifecycle and preset foundations

- [ ] Add backend metadata to infrastructure manifests and a backend resolver in `internal/deploy`.
- [ ] Move current cloud lifecycle behavior behind a `tofuBackend`.
- [ ] Add compatibility metadata to infrastructure and installation manifests.
- [ ] Validate preset compatibility during `init` and `install` before deployment-directory mutation.
- [ ] Extend install-step execution so `localCommand` tasks are passed through alongside `remoteExec`.

## Phase 2: Local runtime foundations

- [ ] Add `internal/localruntime` package scaffolding with deployment-scoped runtime root handling.
- [ ] Add local runtime state model and persistence under `<deploymentDir>/local-runtime/state.json`.
- [ ] Add local port allocation and persisted reuse.
- [ ] Add payload metadata parsing, HTTP download, checksum verification, and cache management.

## Phase 3: macOS arm64 runtime integration

- [ ] Add `internal/localruntime/vm` abstraction with unsupported stubs for non-darwin or non-arm64 builds.
- [ ] Implement darwin/arm64 VM driver using `Virtualization.framework` directly or through a thin wrapper.
- [ ] Implement the mounted control socket/file bridge between host and guest.
- [ ] Add guest bootstrap logic that starts the Linux ExaNano payload under local runtime control.

## Phase 4: Local backend and user-facing behavior

- [ ] Implement `localBackend` for `deploy`, `start`, `stop`, and `destroy`.
- [ ] Add `local` infrastructure preset and dedicated local installation preset such as `nano`.
- [ ] Make `exasol install local` resolve only to compatible installation presets.
- [ ] Generate local-safe `deployment.json`, `secrets.json`, and `connection-instructions.txt`.
- [ ] Make `info`, `connect`, `status`, and `diag info` local-aware.
- [ ] Make `shell host`, `shell container`, and `diag shell` fail with explicit local-unsupported messages.

## Phase 5: Build and release

- [ ] Split macOS arm64 launcher build settings from the generic build matrix.
- [ ] Add signing and notarization support required for the virtualization-enabled launcher.
- [ ] Publish Linux ExaNano payloads as versioned artifacts to a product-owned HTTP location.
- [ ] Update installer and CI/release flows to support local deployment mode on macOS arm64.

## Validation

- [ ] Add unit tests for backend resolution and preset compatibility validation.
- [ ] Add unit tests for local runtime state, payload verification, and port persistence.
- [ ] Add integration tests for local lifecycle commands without cloud credentials.
- [ ] Add concurrent local deployment tests using two deployment directories.
- [ ] Add macOS arm64 platform validation for build, runtime, and installer behavior.
