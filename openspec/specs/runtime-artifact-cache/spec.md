# runtime-artifact-cache Specification

## Purpose
TBD - created by archiving change add-runtime-artifact-cache. Update Purpose after archive.
## Requirements
### Requirement: Runtime artifacts SHALL be cached per user
The launcher SHALL maintain a per-user cache for runtime artifacts that can be reused across launcher operations.

#### Scenario: Runtime artifact is materialized on demand
- **WHEN** a launcher operation requires a runtime artifact that is not present in the user's cache
- **THEN** the launcher materializes the artifact before using it
- **AND** the artifact is recorded in the user's cache metadata

#### Scenario: Cached runtime artifact is reused
- **WHEN** a launcher operation requires a runtime artifact that is already present and valid in the user's cache
- **THEN** the launcher reuses the cached artifact
- **AND** the artifact is not downloaded again

### Requirement: Runtime artifact cache entries SHALL track last use
The launcher SHALL record when each cached runtime artifact was last used.

#### Scenario: New artifact records last use
- **WHEN** a runtime artifact is added to the user's cache
- **THEN** the cache metadata records a last-use timestamp for that artifact

#### Scenario: Cache hit updates last use
- **WHEN** a launcher operation reuses a cached runtime artifact
- **THEN** the cache metadata updates that artifact's last-use timestamp

### Requirement: Runtime artifact changes SHALL refresh cached artifacts
The launcher SHALL treat a cached runtime artifact as invalid when the artifact requested by the launcher no longer matches the artifact recorded in the cache.

#### Scenario: Changed artifact is refreshed
- **WHEN** a launcher operation requires a runtime artifact
- **AND** the user's cache contains an older or different artifact for the same runtime need
- **THEN** the launcher materializes the requested artifact before using it
- **AND** the previous cached artifact remains eligible for cleanup

### Requirement: Runtime artifact downloads SHALL be verified before use
The launcher SHALL verify downloaded runtime artifacts before making them available for use.

#### Scenario: Verification succeeds
- **WHEN** a runtime artifact is downloaded
- **AND** the downloaded artifact matches the expected integrity metadata
- **THEN** the launcher may record and use the artifact

#### Scenario: Verification fails
- **WHEN** a runtime artifact is downloaded
- **AND** the downloaded artifact does not match the expected integrity metadata
- **THEN** the launcher rejects the artifact
- **AND** the rejected artifact is not recorded as usable in the cache

### Requirement: Cache cleanup SHALL use configured retention
The launcher SHALL use per-user cache configuration to determine when cached runtime artifacts are old enough to be cleaned.

#### Scenario: Configured retention identifies stale artifacts
- **WHEN** cache cleanup evaluates cached runtime artifacts
- **THEN** artifacts whose last-use timestamp is older than the configured retention are treated as stale
- **AND** artifacts whose last-use timestamp is within the configured retention are preserved

#### Scenario: Missing retention configuration uses a default
- **WHEN** cache cleanup runs without an existing user retention configuration
- **THEN** the launcher uses a default retention value

### Requirement: Cache cleanup SHALL remove stale artifacts
The launcher SHALL remove stale runtime artifacts and their cache metadata during cleanup.

#### Scenario: Manual cleanup removes stale artifacts
- **WHEN** a user invokes cache cleanup
- **THEN** the launcher removes cached runtime artifacts that are stale
- **AND** the launcher reports the cleanup result

#### Scenario: Automatic cleanup is attempted during cache use
- **WHEN** the launcher successfully uses the runtime artifact cache
- **AND** automatic cleanup is due
- **THEN** the launcher attempts to remove stale cached runtime artifacts

### Requirement: Cache cleanup SHALL support corrupted artifact removal
The launcher SHALL provide a way to remove cached runtime artifacts that fail integrity verification.

#### Scenario: User cleans corrupted artifacts
- **WHEN** a user invokes cache cleanup for corrupted runtime artifacts
- **THEN** the launcher removes cached runtime artifacts that fail integrity verification
- **AND** the launcher reports the cleanup result

### Requirement: Cache cleanup SHALL support full cache removal
The launcher SHALL provide a way to remove all cached runtime artifacts.

#### Scenario: User removes all cached artifacts
- **WHEN** a user invokes cleanup for all cached runtime artifacts
- **THEN** the launcher removes all cached runtime artifacts
- **AND** the launcher reports the cleanup result

#### Scenario: Full cleanup removes unindexed cache contents
- **WHEN** a user invokes cleanup for all cached runtime artifacts
- **AND** the runtime artifact cache contains files or directories not referenced by cache metadata
- **THEN** the launcher removes those unreferenced cache contents
- **AND** the launcher resets runtime artifact cache metadata

### Requirement: Cache cleanup SHALL support partial download removal
The launcher SHALL provide a way to remove partial runtime artifact downloads that were not committed as usable cached artifacts.

#### Scenario: User cleans partial downloads
- **WHEN** a user invokes cache cleanup for partial downloads
- **THEN** the launcher removes partial runtime artifact downloads
- **AND** indexed cached runtime artifacts remain available
- **AND** the launcher reports the cleanup result

### Requirement: Cache cleanup SHALL support previewing cleanup
The launcher SHALL provide a way to preview selected cleanup work without changing cache contents or cache metadata.

#### Scenario: User previews indexed cleanup
- **WHEN** a user invokes cache cleanup in preview mode for cleanup that selects cached runtime artifacts
- **THEN** the launcher reports which indexed cached runtime artifacts would be removed
- **AND** cached runtime artifacts are not removed
- **AND** cache metadata is not changed

#### Scenario: User previews partial download cleanup
- **WHEN** a user invokes cache cleanup for partial downloads in preview mode
- **THEN** the launcher reports which partial runtime artifact downloads would be removed
- **AND** partial runtime artifact downloads are not removed
- **AND** cache metadata is not changed

### Requirement: Cache listing SHALL report cached artifacts
The launcher SHALL provide a way to list cached runtime artifacts.

#### Scenario: User lists cached artifacts
- **WHEN** a user requests the runtime artifact cache contents
- **THEN** the launcher reports each cached runtime artifact
- **AND** the report includes each artifact's last-use timestamp

#### Scenario: Empty cache is listed
- **WHEN** a user requests the runtime artifact cache contents
- **AND** no runtime artifacts are cached
- **THEN** the launcher reports that the cache is empty

### Requirement: Cache unlocking SHALL support stale-lock recovery
The launcher SHALL provide a way to clear a stale runtime artifact cache lock.

#### Scenario: User clears stale cache lock
- **WHEN** a user requests cache unlocking
- **THEN** the launcher clears the cache lock if one exists
- **AND** the launcher reports the unlock result

### Requirement: Cache diagnostics SHALL report cache state without mutation
The launcher SHALL provide runtime artifact cache diagnostics that inspect cache state without changing cache contents or cache locks.

#### Scenario: User inspects cache diagnostics
- **WHEN** a user requests runtime artifact cache diagnostics
- **THEN** the launcher reports cache state information
- **AND** cached runtime artifacts are not removed
- **AND** cache locks are not cleared

#### Scenario: Diagnostics report corrupted artifacts
- **WHEN** a user requests runtime artifact cache diagnostics
- **AND** one or more cached runtime artifacts fail integrity verification
- **THEN** cache diagnostics report those artifacts as corrupted
- **AND** cached runtime artifacts are not removed

#### Scenario: Diagnostics report cache problems
- **WHEN** runtime artifact cache metadata, configuration, integrity, or lock state cannot be interpreted normally
- **THEN** cache diagnostics report the observed problem
- **AND** diagnostics continue reporting any remaining cache state that can be inspected

### Requirement: Runtime artifact cache operations SHALL coordinate concurrent access
The launcher SHALL coordinate operations that read or mutate runtime artifact cache state so concurrent launcher processes do not corrupt cached artifacts or cache metadata.

#### Scenario: Concurrent cache mutation is serialized
- **WHEN** multiple launcher processes attempt to mutate runtime artifact cache state concurrently
- **THEN** only one mutation proceeds at a time

#### Scenario: Cache operation reports lock contention
- **WHEN** a cache operation cannot proceed because another process holds the cache
- **THEN** the launcher reports that the cache is currently locked

