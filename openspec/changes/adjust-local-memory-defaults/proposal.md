## Why

Local deployments currently default VM memory to a fixed 2048 MB. That default does not scale with the host machine and is too small for modern local deployments, starting with macOS.

## What Changes

- Change the local deployment default VM memory on macOS to approximately 50% of total host memory.
- Resolve host memory in the local backend on macOS, where local deployment is currently supported.
- Fail fast when detected host memory is below 8192 MB.
- Reject user-configured local VM memory below 4096 MB.
- Stop hardcoding the local preset memory default to 2048 MB in the embedded infrastructure manifest.
- Update local deployment tests that currently assume a fixed 2048 MB default.

## Capabilities

### New Capabilities

### Modified Capabilities

- `exasol-local-deployment`: local deployment default VM memory changes from a fixed value to a host-memory-based default, with minimum host and minimum configured memory validation

## Impact

- Affected code: local infrastructure preset handling and local backend memory default resolution
- Affected tests: local backend unit tests and local install integration tests
