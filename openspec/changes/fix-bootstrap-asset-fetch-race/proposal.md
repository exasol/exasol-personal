## Why

Bootstrap assets fetched from provider object storage can be unavailable during early cloud-init networking. Cloud-init may then skip a required launcher file, causing installation to fail later with an opaque missing-executable error.

## What Changes

- Defer provider bootstrap asset fetches until cloud-init's final stage.
- Validate that every expected bootstrap asset was written before starting the launcher.
- Report a clear, retryable bootstrap failure when an asset is missing.
- Start the launcher only after deferred bootstrap assets and validation complete.
- Apply the behavior consistently to AWS, Azure, Exoscale, and STACKIT.

## Capabilities

### New Capabilities

- `reliable-bootstrap-asset-delivery`: Ensure provider bootstrap assets are available and validated before launcher startup.

### Modified Capabilities

None.

## Impact

- Changes provider cloud-init rendering and the shared Ubuntu bootstrap sequence.
- Adds no runtime dependencies or changes to provider storage access controls.
- A failed bootstrap is surfaced during cloud-init and requires rerunning the installation.
