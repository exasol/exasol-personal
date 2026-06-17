## Why

Runtime artifacts are currently cached per deployment, so common artifacts are downloaded and stored repeatedly and cannot be managed centrally. A per-user runtime artifact cache reduces duplication, makes artifact reuse visible, and gives users a way to inspect and clean up cached data.

## What Changes

- Add a per-user cache for launcher runtime artifacts.
- Track when each cached artifact was last used so stale artifacts can be identified.
- Add user configuration for the retention period used when cleaning stale cached artifacts.
- Resolve runtime artifacts through the shared cache while preserving artifact validation and refresh behavior when artifact metadata changes.
- Add launcher cache management commands for listing cached artifacts, cleaning stale, corrupted, or partial-download data, previewing cleanup, wiping the cache, and unlocking a stale cache lock.
- Add cache diagnostics that report cache health, integrity, and state without mutating the cache.
- Update documentation to explain cache behavior and management.

## Capabilities

### New Capabilities
- `runtime-artifact-cache`: how the launcher caches, reuses, reports, and cleans per-user runtime artifacts.

### Modified Capabilities
<!-- None: no existing permanent spec covers runtime artifact caching. -->

## Impact

- `internal/runtimeartifacts`: change cache ownership from deployment-local storage to a per-user runtime artifact cache, including metadata, cleanup, and locking.
- `internal/tofu`: resolve OpenTofu through the updated runtime artifact cache abstraction.
- `internal/config` or a new internal cache/config package: resolve user cache/config locations and read cache configuration.
- `cmd/exasol`: add cache management and diagnostic commands.
- Documentation: update launcher architecture/development/user-facing docs where they describe runtime artifacts and cache management.
- Tests: add unit tests for cache metadata, last-use tracking, cleanup, locking, configuration, and command output.
- No new external runtime dependencies are expected.
