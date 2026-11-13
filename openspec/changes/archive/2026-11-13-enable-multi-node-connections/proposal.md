## Why

The unrestricted cloud license permits connections on every database node, but the launcher still selects only the primary node and publishes a single connection address. Users need multi-node connections and complete endpoint information while retaining compatibility with existing deployments and older launchers.

## What Changes

- Expose public node endpoints with explicit connection eligibility in text and JSON connection information.
- Enable multi-node connections for deployments whose saved installation preset declares support, retaining the primary connection for unmarked deployments.
- Verify transport failover and certificate pinning using the existing driver.
- Document multi-host connections and conservative legacy behavior.

## Capabilities

### New Capabilities

- `multi-node-connections`: Connection selection using saved deployment eligibility, public node endpoints, transport failover, and TLS verification.

### Modified Capabilities

- `deployment-info-reporting`: Publish node endpoints and their connection eligibility while retaining existing single-host fields.

## Impact

SPOT-33856 and SPOT-33857 affect installation preset metadata, connection resolution, driver configuration, connection information, tests, user documentation, and the changelog. Existing-deployment license migration and license publishing are outside this change.
