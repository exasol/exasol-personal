## ADDED Requirements

### Requirement: Runtime artifact cache SHALL support fetching from git repository sources
The runtime artifact cache SHALL accept git repository URLs (`git@`, `git://`, `https://*.git`, `http://*.git`) as artifact sources, cloning or updating the repository into the cache.

#### Scenario: Git repository source is cloned on first use
- **WHEN** a git repository source is requested and no cache entry exists for it
- **THEN** the cache SHALL clone the repository content into a new cache entry

#### Scenario: Git repository source is updated on subsequent use
- **WHEN** a git repository source is requested and a cache entry already exists
- **AND** the remote ref has advanced to a new commit
- **THEN** the cache SHALL update the cached clone to the new commit

#### Scenario: Git repository source is pinned by named ref
- **WHEN** a git repository source is requested with a branch or tag ref
- **THEN** the cache SHALL check out the content at that named ref

#### Scenario: Git repository source is pinned by commit SHA
- **WHEN** a git repository source is requested with a full 40-character commit SHA
- **THEN** the cache SHALL check out the content at that exact commit

#### Scenario: Git repository cache entry is reused on same commit
- **WHEN** a git repository source is requested
- **AND** the resolved commit hash matches an existing cache entry
- **THEN** the cache SHALL return the cached path without re-cloning

### Requirement: Runtime artifact cache SHALL identify git artifacts by resolved commit hash
The runtime artifact cache SHALL use the commit hash resolved at fetch time as the content identity for git sources, not a user-supplied checksum.

#### Scenario: Git artifact cache key uses commit hash
- **WHEN** a git artifact is stored in the cache
- **THEN** the cache entry records the resolved commit hash as the content identity

#### Scenario: Git artifact definitions SHALL NOT specify a checksum
- **WHEN** a statically defined git artifact specifies a checksum
- **THEN** the cache SHALL reject the definition with a configuration error

### Requirement: Runtime artifact cache SHALL support fetching from local filesystem sources
The runtime artifact cache SHALL accept `file://` URIs pointing to local directories or archive files as artifact sources.

#### Scenario: file:// directory source is returned directly
- **WHEN** a `file://` URI points to an existing local directory
- **THEN** the cache SHALL return the resolved absolute path directly without copying, extracting, or creating a symlink

#### Scenario: file:// archive source is extracted into the cache
- **WHEN** a `file://` URI points to a local archive file in a supported format
- **THEN** the cache SHALL extract the archive into a cache entry

#### Scenario: file:// path that does not exist returns an error
- **WHEN** a `file://` URI points to a path that does not exist
- **THEN** the cache SHALL return an error that includes the path

### Requirement: Runtime artifact cache SHALL support ZIP archive extraction
The runtime artifact cache SHALL extract `.zip` archives in addition to `.tar.gz`/`.tgz` archives when materialising archive-type artifacts.

#### Scenario: .zip archive artifact is extracted
- **WHEN** an artifact source resolves to a `.zip` archive
- **THEN** the cache SHALL extract the archive contents into the cache entry

#### Scenario: Unsupported archive format returns an error
- **WHEN** an artifact source resolves to a file with an unrecognised archive format
- **THEN** the cache SHALL return an error identifying the unsupported format

### Requirement: Runtime artifact cache SHALL support platform-independent artifact definitions
The runtime artifact cache SHALL accept artifact definitions that use an `"any"` platform key, resolving that definition for any platform when no platform-specific variant is present.

#### Scenario: Platform-independent artifact is resolved for any platform
- **WHEN** an artifact definition specifies only an `"any"` platform key
- **THEN** the cache SHALL resolve that definition regardless of the current platform

#### Scenario: Platform-specific artifact takes precedence over platform-independent
- **WHEN** an artifact definition contains both a platform-specific key and an `"any"` key
- **THEN** the cache SHALL resolve the platform-specific variant for a matching platform

### Requirement: Runtime artifact cache SHALL always re-fetch archive artifacts without a checksum
An archive artifact definition that does not specify a checksum cannot be reliably cached by content identity. The runtime artifact cache SHALL re-fetch such artifacts on every request and SHALL replace any existing cache entry.

#### Scenario: No-checksum archive is re-fetched on every request
- **WHEN** an archive artifact with no checksum is requested
- **THEN** the cache SHALL re-fetch the artifact regardless of any existing cache entry

#### Scenario: No-checksum re-fetch is logged
- **WHEN** an archive artifact with no checksum is re-fetched
- **THEN** the cache SHALL emit a log message indicating the source is being re-fetched because no checksum was specified

### Requirement: Runtime artifact cache SHALL support runtime-constructed artifact definitions
The runtime artifact cache SHALL resolve artifact definitions that are constructed at runtime by callers, without requiring those definitions to be registered in the static resource catalog.

#### Scenario: Runtime-constructed definition is resolved
- **WHEN** a caller supplies an artifact definition directly at resolution time
- **THEN** the cache SHALL resolve and cache the artifact using that definition

#### Scenario: Runtime-constructed definition may omit a checksum
- **WHEN** a runtime-constructed archive artifact definition does not specify a checksum
- **THEN** the cache SHALL resolve the artifact applying the no-checksum re-fetch policy
