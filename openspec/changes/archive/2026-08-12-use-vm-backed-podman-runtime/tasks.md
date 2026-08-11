## 1. Runtime-Neutral Podman Installation

- [x] 1.1 Add direct and runner-backed execution environments with testable command and filesystem operations
- [x] 1.2 Add paired host/runtime artifact paths and Linux identity mapping
- [x] 1.3 Change Nano image loading to atomic materialization plus `podman load -i <runtime-path>`
- [x] 1.4 Route recovery, migration, reports, and custom SLC imports through the installation environment
- [x] 1.5 Extend unit tests and verify unchanged Linux host Podman behavior

## 2. VM-Backed macOS Runtime

- [x] 2.1 Implement the v2 runner command/state contract and reject legacy runners before startup
- [x] 2.2 Stage and map Nano and custom SLC artifacts through the VM shared directory
- [x] 2.3 Start the VM with labeled application forwards, run the shared Podman installation inside it, and derive effective endpoints
- [x] 2.4 Order macOS status, health, stop, destroy, diagnostics, and legacy cleanup around VM availability
- [x] 2.5 Add macOS runtime unit and workflow tests using a fake v2 runner

## 3. Shell and Metadata Integration

- [x] 3.1 Delegate local host and container shells to the selected runtime
- [x] 3.2 Remove SSH endpoint and key dependencies from new macOS deployment metadata and workflows
- [x] 3.3 Preserve existing cloud metadata and explicit Linux shell behavior

## 4. Verification and Documentation

- [x] 4.1 Run unit, lint, and Linux local smoke tests
- [x] 4.2 Build exasol-personal for Darwin ARM64 and run a macOS end-to-end deployment with the v2 runner override
- [x] 4.3 Update architecture, runtime prerequisites, and changelog documentation
- [x] 4.4 Validate and archive the completed OpenSpec change
