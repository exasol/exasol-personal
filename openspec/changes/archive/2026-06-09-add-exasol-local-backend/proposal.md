## Why

Exasol Personal can already deploy to remote infrastructure, but local development on Apple Silicon macOS depends on the Exasol Local runner managing its own runtime outside the launcher contract. Bringing that local runtime under the launcher makes `exasol install local`, lifecycle commands, SQL connection, and shell access behave like the existing deployment workflows.

## What Changes

- Add a local infrastructure backend that stages an embedded macOS Apple Silicon Exasol Local runner.
- Add a compatible local installation path for the Exasol Local database container instead of the existing remote-exec Ubuntu installation flow.
- Make the launcher own the local VM runtime lifetime, including initialization, start, stop, and destruction.
- Make the launcher own a managed deployment share used for SSH key injection and runner/guest coordination.
- Produce normal launcher deployment artifacts for local deployments, including `deployment.json`, `secrets.json`, and connection instructions.
- Support `exasol connect` against the locally forwarded Exasol Local database endpoint using `sys` / `exasol` credentials for the initial version.
- Support `exasol shell host` into the local VM and `exasol shell container` into the Exasol Local database container.
- Make `exasol destroy` remove local VM disk/data and launcher-owned local runtime artifacts.

## Capabilities

### New Capabilities

- `local-exasol-deployment`: Exasol Local VM deployments managed through the standard launcher lifecycle, connection, and shell commands.

### Modified Capabilities

None.

## Impact

- Adds a non-Tofu deployment backend and local infrastructure preset.
- Adds an Exasol Local installation preset compatible with the new backend.
- Touches deployment lifecycle handling, backend resolution, connection artifact generation, shell behavior, and tests.
- Depends on the existing macOS Exasol Local runner contract: `init`, `start <cpus> <memory_mb> <data_size_gb>`, `stop`, managed shared directory behavior, and `vm-state.json` with forwarded ports. The runner binary is provided as a build-time embedded asset.
