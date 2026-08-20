## 1. Mount-aware legacy adoption

- [x] 1.1 Inspect the `/exa` mount source and destination and skip migration only when the source is the configured persistent data directory.
- [x] 1.2 Remove the legacy-name forced migration mode while preserving migration for overlay-backed and differently mounted data.
- [x] 1.3 Add unit coverage for adopting a legacy-named container already mounted from the populated persistent data directory.

## 2. macOS test lifecycle and contracts

- [x] 2.1 Add unconditional teardown to standalone local deployment and VM chaos tests.
- [x] 2.2 Replace copied-runner and SSH-key assertions with the loopback endpoint, shell support, no-nodes, and no-SSH-transport contract.

## 3. Local storage durability

- [x] 3.1 Add the OpenPGP build tag to Terraform formatting so repository validation works without system GPGME bindings.
- [x] 3.2 Add execution-environment synchronization after image preparation and before Nano starts on Linux and macOS.
- [x] 3.3 Add an explicitly named Nano startup durability workaround after database readiness on Linux and macOS.
- [x] 3.4 Cover ordering, platform delegation, synchronization failures, and committed-data recovery.

## 4. Verification and specification lifecycle

- [x] 4.1 Run formatting, build, lint, and unit tests on Linux for every rewritten commit.
- [x] 4.2 Validate the active OpenSpec change.
- [x] 4.3 Synchronize the delta into the main specification and archive the change in a separate commit after implementation stabilizes.
