## 1. Cache paths and configuration

- [x] 1.1 Add helpers that resolve the per-user runtime artifact cache root under the operating system user cache directory with a `.exasol/personal/runtime-artifacts` namespace
- [x] 1.2 Add helpers that resolve the launcher configuration directory from the launcher root directory
- [x] 1.3 Implement cache retention configuration loading with a default `retention_days` value when configuration is missing
- [x] 1.4 Implement default configuration file creation when an explicit cache command initializes cache configuration
- [x] 1.5 Add validation and clear errors for invalid retention configuration

## 2. Cache index and metadata

- [x] 2.1 Add a runtime artifact cache index model with schema version, last cleanup timestamp, and artifact entries
- [x] 2.2 Add cache entry fields for logical artifact ID, platform, source metadata, relative artifact path, relative resolved path, created timestamp, last-use timestamp, and size
- [x] 2.3 Implement artifact identity derivation from logical artifact ID, platform, source URL, checksum, extraction setting, download path, and resource path
- [x] 2.4 Implement cache index read/write with missing-file defaults, JSON validation, relative path containment checks, and atomic writes
- [x] 2.5 Add a test clock abstraction so timestamp and cleanup behavior can be tested deterministically

## 3. Cache locking

- [x] 3.1 Add a runtime artifact cache lock helper that creates the cache directory before constructing `internal/directorymutex`
- [x] 3.2 Use exclusive locks for cache materialization, metadata updates, listing, cleanup, and unlock operations
- [x] 3.3 Use a cache-specific lock acquisition timeout suitable for download/extraction waits and release locks with a cancellation-independent context
- [x] 3.4 Map cache lock acquisition failures to clear user-facing errors
- [x] 3.5 Implement cache lock status inspection for diagnostics without acquiring or mutating the lock
- [x] 3.6 Implement force-clearing of stale cache locks for the cache unlock command

## 4. Runtime artifact manager

- [x] 4.1 Refactor `internal/runtimeartifacts.Manager` so production construction uses the per-user cache instead of a deployment directory
- [x] 4.2 Keep test constructors that allow explicit cache roots, platform values, and clock injection
- [x] 4.3 Update the request flow to compute artifact identity, acquire the cache lock, read the index, and validate cache hits
- [x] 4.4 Update cache hits to refresh `lastUsedAt`, persist metadata, and return the resolved path
- [x] 4.5 Update cache misses and invalid hits to download to a temporary location, verify checksum, extract when required, and commit files plus metadata atomically
- [x] 4.6 Preserve existing path containment, checksum mismatch, platform resolution, download path, and archive extraction behavior
- [x] 4.7 Implement opportunistic automatic cleanup after successful artifact use when cleanup is due
- [x] 4.8 Update `internal/tofu.ResolveBinaryPath` to use the new default runtime artifact manager
- [x] 4.9 Stage runtime artifact downloads under the cache's download staging area

## 5. Cache operations

- [x] 5.1 Implement a cache listing operation that returns cached artifact entries with last-use timestamps and size information
- [x] 5.2 Implement a manual cleanup operation that removes stale artifacts and metadata according to the configured retention
- [x] 5.3 Implement corrupted artifact detection by checksumming cached downloaded artifacts against their expected checksums
- [x] 5.4 Implement a manual cleanup mode that removes corrupted artifacts and metadata on explicit request
- [x] 5.5 Implement a manual cleanup mode that removes all cached runtime artifacts and resets artifact metadata
- [x] 5.6 Implement dry-run cleanup planning for stale, invalid-artifact, and full-cache cleanup without mutating files or metadata
- [x] 5.7 Implement cleanup summaries with removed entry count, removed byte count, corrupted entry count when applicable, and dry-run wording when applicable
- [x] 5.8 Implement diagnostic cache inspection that reports cache root, config status, index status, lock status, entry count, size, checksum status, stale candidates, missing referenced files, and unexpected unreferenced files where practical
- [x] 5.9 Ensure diagnostic inspection does not remove artifacts, rewrite metadata, or clear locks
- [x] 5.10 Implement partial-download cleanup and include staged downloads in full-cache cleanup

## 6. CLI wiring

- [x] 6.1 Add a top-level `cache` command group that does not register deployment-directory flags or deployment compatibility checks
- [x] 6.2 Add `cache list` with text output and JSON output support
- [x] 6.3 Add `cache clean` with text summary output for stale cleanup
- [x] 6.4 Add an explicit `--invalid` cleanup option to `cache clean`
- [x] 6.5 Add an explicit `--all` cleanup option to `cache clean`
- [x] 6.6 Add a `--dry-run` option to `cache clean` that previews the selected cleanup mode
- [x] 6.7 Make `--invalid` and `--all` mutually exclusive while allowing `--dry-run` with any cleanup mode
- [x] 6.8 Add `cache unlock` with help text warning users to unlock only when no launcher process is using the cache
- [x] 6.9 Add `diag cache` as a read-only diagnostic command
- [x] 6.10 Verify cache and diagnostic cache commands do not start deployment log sessions or emit default deployment directory messages
- [x] 6.11 Add `cache clean --partial-downloads` and keep cleanup selectors mutually exclusive

## 7. Tests

- [x] 7.1 Add unit tests for cache path/config resolution, default retention, invalid retention, and default config creation
- [x] 7.2 Add unit tests for cache index read/write, artifact identity, relative path containment, and atomic metadata updates
- [x] 7.3 Add runtime artifact manager tests for first materialization, cache hit reuse, last-use updates, artifact identity changes, missing file refresh, checksum mismatch, and archive extraction
- [x] 7.4 Add cleanup tests for stale vs retained artifacts, corrupted artifacts, full-cache removal, dry-run behavior, automatic cleanup gating, metadata removal, file removal, and cleanup summaries
- [x] 7.5 Add diagnostic tests for checksum match and checksum mismatch reporting
- [x] 7.6 Add locking tests for exclusive serialization, lock contention errors, diagnostic lock status, and force unlock behavior
- [x] 7.7 Add CLI tests for `cache list`, `cache list --json`, `cache clean`, `cache clean --invalid`, `cache clean --all`, `cache clean --dry-run`, invalid cleanup-mode combinations, `cache unlock`, and `diag cache`
- [x] 7.8 Add tests that cache commands do not require or resolve a deployment directory
- [x] 7.9 Add tests for download staging location, partial-download cleanup, full-cache cleanup of staged downloads, and CLI partial-download cleanup output

## 8. Documentation and verification

- [x] 8.1 Update architecture and development documentation to describe per-user runtime artifact caching instead of deployment-local runtime artifact storage
- [x] 8.2 Update user-facing command documentation/help examples for cache listing, cleaning, unlocking, and diagnostics
- [x] 8.3 Run formatting for touched Go files
- [x] 8.4 Run focused Go unit tests for runtime artifacts, cache commands, diagnostics, and Tofu resource resolution
- [x] 8.5 Run the repository's standard unit-test and lint checks; document any pre-existing unrelated failures
- [x] 8.6 Update cache management documentation for partial-download cleanup
