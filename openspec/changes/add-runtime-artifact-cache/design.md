## Context

The launcher currently resolves runtime artifacts through `internal/runtimeartifacts`.
The manager receives a deployment directory, stores artifacts below that deployment, and records cache metadata in a deployment-local `resources.json`. OpenTofu is the current runtime artifact, resolved by `internal/tofu.ResolveBinaryPath` from the embedded `assets/resources/resources.yaml`.

That design works, but it duplicates the same downloaded and extracted artifact across deployments. It also gives users no central way to see which runtime artifacts exist, when they were last used, or how much cache space they consume.

This change turns runtime artifact storage into a per-user cache. The existing resource-definition model remains the source of truth for which artifact is required for the current platform. The cache becomes the place where materialized artifacts and their use metadata live.

## Goals / Non-Goals

**Goals:**
- Store runtime artifacts in a per-user cache rooted under a launcher-owned directory in the operating system user cache location.
- Use the term `runtime-artifacts` for the cache namespace to align with the existing `runtimeartifacts` package.
- Track last-use timestamps for all cached runtime artifacts.
- Clean stale artifacts based on a per-user retention configuration.
- Detect cached artifacts whose downloaded artifact no longer matches the expected checksum.
- Provide user-facing cache management commands for listing, cleaning, and unlocking the cache.
- Provide a diagnostic command that reports cache state without mutating it.
- Coordinate concurrent launcher processes that may resolve or clean the shared cache.
- Preserve checksum validation, platform selection, archive extraction, and artifact-refresh semantics.

**Non-Goals:**
- Do not introduce generalized package management for arbitrary third-party tools.
- Do not add support for artifacts beyond the existing runtime artifact definition model.
- Do not change deployment-directory compatibility or deployment state semantics.
- Do not move unrelated cache data, such as SQL shell history, as part of this change.

## Decisions

### Use a named per-user cache namespace

Runtime artifact cache data will live under the operating system user cache directory, inside a `.exasol/personal/runtime-artifacts` namespace. The concrete root is resolved at runtime with `os.UserCacheDir()` and path joining, so platform-specific cache base locations remain delegated to Go and the host operating system.

Inside that namespace:
- `index.json` stores cache metadata.
- `artifacts/` stores materialized artifacts.
- `downloads/` stores in-progress artifact downloads and extraction staging.
- directory mutex marker files may appear while an operation holds the cache lock.

Rationale:
- `.exasol/personal` mirrors the launcher-owned durable configuration layout while staying under the operating system cache location.
- `runtime-artifacts` aligns with the existing package and feature vocabulary.
- The namespace leaves room for other launcher cache data later without mixing it with runtime artifact metadata.

Alternative considered: store artifacts directly under the user cache directory. Rejected because it makes the cache harder to identify and leaves no clean namespace for future cache data.

### Store retention settings in launcher configuration

Cache retention configuration will live in the launcher's durable local root directory. The implementation should reuse the same root-directory helper used to derive the default deployment directory rather than using `os.UserConfigDir()`. The cache data remains in the operating system user cache directory, but user settings should not live in a directory that the operating system or a user may purge as disposable cache data.

The configuration file will contain a positive retention value:

```yaml
retention_days: 30
```

The initial default is 30 days. If the configuration file is missing, cache management will use the default and create the default file when appropriate. Invalid configuration is reported clearly by explicit cache commands. Runtime artifact resolution should not fail solely because automatic cleanup configuration is invalid; in that case, artifact resolution should proceed and skip automatic cleanup with a warning.

Rationale:
- `retention_days` is shorter than `cleanup_after_days` while still describing the cache policy and making the unit explicit.
- An integer day count is easy for users to edit and avoids ambiguity in duration parsing.
- Go's built-in duration parser does not support day units, so a field such as `cleanup_after: 30d` would require custom parsing.
- Keeping configuration outside the cache prevents a cache purge from erasing user preferences.
- Using the launcher root directory keeps launcher-owned configuration near the launcher's existing durable local state.

Alternative considered: place `config.yaml` inside the runtime artifact cache directory. Rejected because cache directories are disposable by convention.

Alternative key names considered:
- `max_age_days`: concise, but emphasizes deletion threshold more than cache policy.
- `expire_after_days`: clear, but still fairly verbose.
- `keep_days`: short, but less precise about whether it means minimum or maximum retention.
- `ttl_days`: concise, but more technical and less friendly for end users.
- `retention`: shortest, but would either hide the unit or require a custom duration parser.

### Key cache entries by artifact identity

The cache index will store entries keyed by a stable artifact identity derived from:
- logical runtime artifact ID
- platform
- source URL
- expected checksum
- extraction flag
- download path override
- resource path within an archive

The key should be a filesystem-safe digest rather than raw user-visible metadata. Artifact files should be stored under a layout grouped by logical artifact ID and platform, with the digest as the final identity component.

Rationale:
- A shared cache can hold multiple versions of the same logical artifact at the same time.
- URL or checksum changes produce a distinct cache entry, preserving the current behavior that artifact metadata changes refresh the artifact.
- Grouping by logical ID and platform keeps manual inspection understandable while avoiding path collisions.

Alternative considered: key only by logical artifact ID. Rejected because different platforms, versions, or URLs would overwrite each other.

### Keep index paths relative to the cache root

`index.json` will store artifact and resolved paths relative to the runtime artifact cache root. Runtime code will resolve those paths under the cache root and reject any path that escapes that root.

Rationale:
- Relative paths keep metadata stable if the cache root is relocated by test setup or operating system behavior.
- Containment checks preserve the existing safety property used for download and resource paths.

### Record last use on every successful artifact request

Every successful runtime artifact request will set `lastUsedAt` to the current UTC time before returning the resolved artifact path. New entries will also record `createdAt`.

Timestamps should be serialized in RFC3339 format. A clock abstraction should be used in tests so last-use and cleanup behavior are deterministic.

Rationale:
- Cache cleanup depends on reliable last-use metadata.
- Updating last use on cache hits keeps active artifacts from being removed by cleanup.

### Reuse `internal/directorymutex` with exclusive cache locks

The runtime artifact cache will use `internal/directorymutex` to coordinate cross-process access. The cache directory must be created before constructing the mutex because `directorymutex.New` requires an existing directory.

Use an exclusive lock for all mutating cache operations:
- runtime artifact materialization and cache-hit metadata updates
- manual cleanup
- configuration writes, if introduced by a cache command

Use an exclusive lock for cache listing in the first version too, even though listing is conceptually read-only. The current shared-lock stress test in `internal/directorymutex` is skipped as flaky, so depending on shared mode is unnecessary risk for a cache whose operations are infrequent.

Force unlocking deliberately clears the stale cache lock marker without acquiring the cache lock first, because it is the recovery path for cases where normal lock acquisition is blocked.

Diagnostic cache inspection should not take the cache lock. It should report the observed lock status and read metadata best-effort. This avoids blocking diagnostics when the cache is locked, which is the case diagnostics often need to explain.

The cache lock acquisition timeout should be longer than the deployment-directory default because another launcher process may be downloading and extracting a runtime artifact. A cache-specific helper should wrap the caller context with a practical timeout when none is present. Lock release should use a context that is not canceled by command cancellation, matching the deployment lock pattern.

Rationale:
- The existing directory mutex already provides marker-file based cross-process locking and force-clear support.
- Exclusive-only cache access keeps the first version simple and avoids relying on flaky shared-lock behavior.
- A longer timeout avoids failing concurrent launches while the first process is warming the cache.

Alternative considered: use OS-specific file locks. Rejected because the repository already has a portable lock abstraction that fits this directory-oriented cache.

### Add cache unlocking as a user-facing maintenance operation

The cache management command set will include an unlock operation that force-clears the cache lock marker. The help text must warn that users should only unlock when they are certain no launcher process is using the cache.

Rationale:
- `directorymutex` markers can remain after process crashes.
- A shared cache can block unrelated deployments if a stale lock is left behind.
- Deployment directories already have a similar operational model through diagnostic unlocking.

### Add read-only cache diagnostics

`exasol diag cache` will report cache state without mutating the cache. It should include:
- cache root path
- configuration file path
- parsed retention value or configuration error
- index file path
- index parse status
- lock status from `directorymutex.Status()`
- artifact entry count
- approximate total artifact size
- checksum status for cached downloaded artifacts
- stale candidate count using the configured retention when available
- missing files referenced by metadata
- unexpected files or directories that are not referenced by metadata, where practical

Checksum diagnostics should compute the checksum of the cached downloaded artifact and compare it with the expected checksum recorded in metadata. The check applies to the downloaded artifact, not the extracted resolved path, matching the runtime artifact verification model. Diagnostics should report mismatches as corrupted artifacts and include enough entry metadata for the user to identify what will be cleaned.

The diagnostic command should succeed with a useful report when the cache is missing, locked, or partially invalid. It should fail only when the underlying environment prevents even basic inspection.

Rationale:
- Users and support engineers need a non-destructive way to understand cache problems before cleaning or unlocking.
- Diagnostics should be useful exactly when regular cache operations are blocked.
- Reporting checksum mismatches helps distinguish missing files, stale entries, and corrupted cached artifacts before a user chooses a cleanup action.

### Add cache management commands

The cache command group will expose:
- `exasol cache list`: list cached artifacts and last-use timestamps.
- `exasol cache clean`: remove artifacts older than the configured retention and report what was removed.
- `exasol cache clean --invalid`: remove artifacts whose cached downloaded artifact no longer matches the expected checksum.
- `exasol cache clean --all`: remove all cached runtime artifacts and reset cache artifact metadata.
- `exasol cache clean --partial-downloads`: remove staged downloads that were not committed as usable cached artifacts.
- `exasol cache clean --dry-run`: preview indexed artifacts selected by the cleanup mode without deleting artifacts or changing metadata.
- `exasol cache unlock`: force-clear a stale cache lock.

`cache list` should support the existing `--json` output pattern for automation. Text output should include logical artifact ID, platform, last-used timestamp, human-friendly size, and resolved path. Reported size should represent the full cached entry; for extracted artifacts this includes both the downloaded archive and extracted contents. Paths that are inside the runtime artifact cache should be displayed relative to the cache root, so reports remain readable while the cache root is still shown explicitly. `cache clean` should print a summary with removed entry count and human-friendly removed size; when invalid-artifact cleanup is requested, the summary should also report how many removed entries failed checksum verification. During dry runs, the summary should use "would remove" wording, report the indexed artifacts selected by cache metadata for metadata-based modes, and must not mutate files or metadata. Partial-download dry runs should report staged partial downloads selected for removal. JSON output may be added if it fits the existing command patterns during implementation.

`--invalid`, `--all`, and `--partial-downloads` are cleanup selectors and should be mutually exclusive. With no selector, `cache clean` selects stale cleanup. `--dry-run` is not a selector; it can be combined with stale cleanup, invalid-artifact cleanup, full-cache cleanup, or partial-download cleanup.

Cache commands do not operate on a deployment directory, so they should not register `--deployment-dir`, enforce deployment compatibility, or start deployment log sessions.

Rationale:
- User-facing maintenance belongs under `cache`.
- Diagnostics belongs under `diag` and remains read-only.
- Keeping these commands independent from deployment directories makes cache maintenance possible even when no deployment exists.

### Keep cleanup conservative and bounded

Manual cleanup removes entries whose `lastUsedAt` is older than the configured retention. When the user requests invalid-artifact cleanup, manual cleanup removes entries whose cached downloaded artifact fails checksum verification. When the user requests partial-download cleanup, manual cleanup removes staged downloads that were not committed as usable cached artifacts and leaves indexed artifacts unchanged. When the user requests full-cache cleanup, manual cleanup wipes the runtime artifact cache contents, including files not referenced by metadata and staged partial downloads, and resets artifact metadata while preserving cache configuration. Cleanup removes files that belong to removed entries and removes the corresponding index entries.

Dry-run cleanup computes the same metadata-based cleanup plan as the selected cleanup mode and reports the indexed artifacts selected by that plan without deleting files, changing cache metadata, or updating cleanup timestamps. For full-cache cleanup, the applied cleanup also removes unindexed cache contents, but dry-run reporting remains limited to indexed cache entries.

Automatic cleanup should run opportunistically after a successful runtime artifact request, not before returning a path. The index should record `lastCleanupAt`, and automatic cleanup should run only when the previous cleanup is older than a bounded interval, such as 24 hours. Automatic cleanup should remove stale artifacts only; invalid-artifact cleanup remains explicit because it requires checksum reads over cached artifacts and is a maintenance action. Automatic cleanup failures should be logged but should not fail the artifact request.

Rationale:
- Updating last use before cleanup prevents the just-used artifact from becoming a cleanup candidate.
- Bounding automatic cleanup avoids making every launcher operation scan the whole cache.
- Manual cleanup remains the strict path for users who explicitly asked to clean the cache.
- Invalid-artifact cleanup is explicit so normal launcher operations do not pay the cost of checksumming every cached artifact.
- Full-cache cleanup is explicit because it removes artifacts that may still be within the retention window.
- Dry-run cleanup lets users inspect indexed artifacts selected for cleanup before applying it.

Alternative considered: clean on every artifact request. Rejected because it adds unnecessary filesystem scanning to normal launcher operations.

### Preserve runtime artifact validation behavior

Cache hits are valid only when:
- cache metadata matches the requested artifact identity
- the downloaded artifact path exists
- the resolved path exists when extraction is enabled

Cache misses or invalid hits should refresh the artifact by downloading to the cache's download staging area, verifying the checksum, extracting when required, and atomically committing metadata after files are in place.

Rationale:
- This preserves the existing download, checksum, and extraction safety model.
- It keeps stale or partial downloads from becoming visible as usable artifacts.

## Risks / Trade-offs

- [A stale cache lock can block runtime artifact resolution] -> Provide `exasol cache unlock`, document when it is safe, and expose lock state through `exasol diag cache`.
- [Exclusive locking can serialize independent cache operations] -> Accept for the first version because runtime artifact operations are infrequent and shared locking has known test instability.
- [Holding the cache lock during download can block other launchers] -> Use a longer cache lock timeout and keep the implementation simple; optimize with per-entry locks later if runtime artifacts grow significantly.
- [Checksum diagnostics and invalid-artifact cleanup can be expensive for large caches] -> Run full checksum verification only for explicit diagnostics and invalid-artifact cleanup, not for normal listing or automatic cleanup.
- [Full-cache cleanup can remove artifacts that are still useful] -> Require an explicit `--all` selector and support `--dry-run` so users can preview the impact.
- [Invalid user configuration could affect deployment commands] -> Do not fail runtime artifact resolution solely because automatic cleanup configuration is invalid; report invalid config in cache commands and diagnostics.
- [Manual unlock can be misused while another process is active] -> Make command help explicit and keep diagnostics available so users can inspect before unlocking.

## Migration Plan

1. Add the new cache path/config resolution helpers and runtime artifact cache data model.
2. Refactor `internal/runtimeartifacts.Manager` so production construction resolves the per-user cache rather than requiring a deployment directory.
3. Update `internal/tofu.ResolveBinaryPath` to use the default runtime artifact cache.
4. Add cache management and diagnostic commands.
5. Update docs that currently state runtime artifacts are stored in deployment directories.

Rollback is straightforward because the change does not modify deployment state. Reverting the launcher returns runtime artifact resolution to deployment-local storage; any per-user cache files become unused local cache data.

## Open Questions

None.
