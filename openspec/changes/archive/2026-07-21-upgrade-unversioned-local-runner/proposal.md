## Why

Existing Exasol Local deployments retain the runner first staged into their runtime directory, so launcher releases cannot deliver runner bug fixes or migrations. The runner shipped with Exasol Personal 2.0 has no version command, requiring a bounded legacy upgrade rule before future runners can use semantic-version compatibility.

## What Changes

- Reconcile the launcher-managed local runner immediately before initializing or starting a stopped local VM.
- Replace an unversioned legacy runner with the embedded migration-capable runner.
- Use runner semantic versions to automatically apply newer patch and minor releases without downgrading.
- Preserve an installed runner across major-version differences and report an actionable incompatibility instead of updating automatically.
- Replace differing bytes for the same runner version from the trusted embedded copy.
- Provide an internal development override that forces byte-based reconciliation when the embedded runner is temporarily unversioned.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `exasol-local-deployment`: Define version-aware staging, the bounded unversioned-runner migration, and major-version protection for launcher-owned local runners.

## Impact

The local runtime staging path, embedded runner metadata, local lifecycle diagnostics, fake-runner tests, and release packaging are affected. The runner contract gains a side-effect-free `version` command returning a semantic version; no cloud deployment behavior changes.
