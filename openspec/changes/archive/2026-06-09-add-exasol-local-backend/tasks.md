## 1. Presets and Backend Plumbing

- [x] 1.1 Add an embedded `local` infrastructure preset with `backend: local` and a local capability declaration.
- [x] 1.2 Add an embedded Exasol Local installation preset compatible with the local backend and without remote-exec installation steps.
- [x] 1.3 Extend backend resolution to recognize `backend: local` while preserving existing `tofu` behavior.
- [x] 1.4 Add local backend environment validation that accepts macOS Apple Silicon and rejects unsupported platforms before VM startup.

## 2. Runner and Runtime State

- [x] 2.1 Add a local runner adapter that stages the embedded `mac-runner-aarch64` and invokes it from a launcher-owned runtime directory.
- [x] 2.2 Define launcher-owned local runtime paths for VM files, runner state, managed share, SSH key material, and connection artifacts.
- [x] 2.3 Generate local SSH key material and write the public key to the managed share as `authorized_keys`.
- [x] 2.4 Parse runner `vm-state.json` and validate forwarded SSH, database, and UI ports.
- [x] 2.5 Write local `deployment.json` using loopback endpoints, forwarded ports, shell support, and insecure certificate validation metadata.
- [x] 2.6 Write local `secrets.json` with `sys` / `exasol` database credentials.
- [x] 2.7 Expose Exasol Local VM CPU, memory, and data disk sizing through the runner start contract.

## 3. Lifecycle Implementation

- [x] 3.1 Implement local deploy to initialize the runner runtime, prepare the managed share, start the VM, write artifacts, and wait for database readiness.
- [x] 3.2 Implement local start to start the VM, refresh forwarded port metadata, rewrite connection artifacts, and wait for database readiness.
- [x] 3.3 Implement local stop to stop the VM and let the existing workflow state handling record the deployment as stopped.
- [x] 3.4 Implement local destroy to stop the VM if needed, delete VM disk/data and launcher-owned runtime files, remove connection artifacts, and return the deployment to initialized.

## 4. Connection and Shell Behavior

- [x] 4.1 Verify `exasol connect`, `status`, and `info` consume local artifacts without cloud-specific assumptions.
- [x] 4.2 Implement local `shell host` through the forwarded SSH endpoint using the generated key.
- [x] 4.3 Implement local `shell container` by opening an interactive shell in the Exasol Local database container, with a fallback from `bash` to `sh`.
- [x] 4.4 Update local connection instruction text where needed so destroy semantics and local shell behavior are clear.

## 5. Tests and Documentation

- [x] 5.1 Add unit tests for backend resolution and unsupported-platform validation.
- [x] 5.2 Add unit tests for runner state parsing, local deployment artifact generation, and local credential writing.
- [x] 5.3 Add unit tests for local destroy cleanup behavior using fake runner/runtime files.
- [x] 5.4 Add CLI/integration coverage for local preset selection using a fake or test runner path so tests do not require a real VM.
- [x] 5.5 Update user-facing documentation for `exasol install local`, local credentials, managed share scope, shell commands, and destructive local destroy behavior.
- [x] 5.6 Run formatting, unit tests, and targeted integration tests according to the development guide.
