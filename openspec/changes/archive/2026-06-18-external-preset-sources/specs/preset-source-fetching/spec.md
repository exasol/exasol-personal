## ADDED Requirements

### Requirement: Git and checksummed archive preset sources are cached locally
A resolved git preset source or an archive preset source with a checksum SHALL be stored in the local resource cache so that subsequent uses of the same source at the same content version do not require re-fetching. Archive sources without a checksum are not cached.

#### Scenario: Cached git preset at the same commit is reused
- **WHEN** the user invokes a command with a git preset source that resolves to a commit already in the cache
- **THEN** the system SHALL use the cached copy without re-cloning

#### Scenario: Cached checksummed archive is reused
- **WHEN** the user invokes a command with an archive preset source whose URL and checksum match a cache entry
- **THEN** the system SHALL use the cached copy without re-downloading

#### Scenario: Cache miss triggers a fresh fetch
- **WHEN** no matching cache entry exists for the given preset source
- **THEN** the system SHALL fetch the preset and store it in the cache before use

### Requirement: Local file:// preset sources are handled according to their content kind
A `file://` URI pointing to a directory SHALL be used as-is without copying, extracting, or caching. A `file://` URI pointing to a supported archive file SHALL be extracted into the cache in the same way as a remote archive. A `file://` URI that is neither a directory nor a supported archive SHALL be rejected with an error.

#### Scenario: file:// directory is used directly without caching
- **WHEN** the user specifies a `file://` URI pointing to a local directory
- **THEN** the system SHALL use that directory as the preset without any caching or copying

#### Scenario: file:// archive is extracted into the cache
- **WHEN** the user specifies a `file://` URI pointing to a supported local archive file
- **THEN** the system SHALL extract the archive into the local cache and use the extracted contents as the preset

### Requirement: Git repository sources are resolved without an external git tool
A preset source identified as a git repository SHALL be fetched using a built-in git implementation, with no dependency on an external git binary.

#### Scenario: Git repository preset is fetched successfully
- **WHEN** the user specifies a reachable git repository URL as a preset source
- **THEN** the system SHALL fetch the repository content and make it available as a local preset directory

#### Scenario: Unreachable git repository returns a clear error
- **WHEN** the specified git repository URL is unreachable or invalid
- **THEN** the system SHALL return an error that includes the URL

### Requirement: Git repository cache identity is based on resolved commit hash
A cached git preset entry SHALL be identified by the commit hash resolved from the specified ref. Two requests for the same URL that resolve to the same commit SHALL reuse the same cache entry; a URL that resolves to a new commit SHALL be fetched and cached separately.

#### Scenario: Same commit hash reuses cache
- **WHEN** a git preset URL is requested and a cache entry already exists for the same URL and commit hash
- **THEN** the system SHALL use the cached entry without re-fetching

#### Scenario: New commit hash bypasses cache
- **WHEN** a git preset URL resolves to a commit hash not present in the cache
- **THEN** the system SHALL fetch and cache the new content

### Requirement: Remote archive preset sources without a checksum are always re-fetched
A remote archive preset source that does not specify a checksum cannot be cached reliably. The system SHALL re-download such a source on every invocation and SHALL log a message indicating that the source is being re-fetched because no checksum is available.

#### Scenario: No-checksum archive source is re-downloaded every time
- **WHEN** the user specifies a remote archive preset source with no checksum
- **THEN** the system SHALL download the archive on each invocation and SHALL log a message stating the reason

#### Scenario: No-checksum re-fetch replaces any prior cache entry
- **WHEN** a no-checksum archive source is re-downloaded
- **THEN** the system SHALL replace any existing cache entry for that URL with the newly downloaded content

### Requirement: Archive sources support tar.gz and zip formats
An archive preset source, whether a remote URL or a local `file://` path, SHALL be accepted in `.tar.gz`/`.tgz` or `.zip` format. Unrecognised formats SHALL be rejected with an error.

#### Scenario: .tar.gz archive is accepted
- **WHEN** the source URL or local path refers to a `.tar.gz` or `.tgz` archive
- **THEN** the system SHALL extract the archive contents into the cache

#### Scenario: .zip archive is accepted
- **WHEN** the source URL or local path refers to a `.zip` archive
- **THEN** the system SHALL extract the archive contents into the cache

#### Scenario: Unsupported archive format returns an error
- **WHEN** the source URL or local path refers to a file with an unrecognised or unsupported format
- **THEN** the system SHALL return an error describing the unsupported format

### Requirement: Unreachable sources return a clear error
The system SHALL return a descriptive error when any external preset source cannot be reached or does not exist.

#### Scenario: Unreachable remote source returns an error
- **WHEN** the remote URL is unreachable or returns an error response
- **THEN** the system SHALL return an error that includes the source URL

#### Scenario: Non-existent local path returns an error
- **WHEN** the path encoded in a `file://` URI does not exist on the filesystem
- **THEN** the system SHALL return an error that includes the resolved path

#### Scenario: file:// path is a non-archive file
- **WHEN** the path encoded in a `file://` URI exists but is neither a directory nor a supported archive file
- **THEN** the system SHALL return an error stating that the path must be a directory or a supported archive

### Requirement: Statically defined archive resources require a checksum
An archive resource defined in the static resource catalog SHALL specify a checksum. The system SHALL reject any static resource definition that omits a checksum for an archive source.

#### Scenario: Static archive resource without checksum is rejected at load time
- **WHEN** the static resource catalog contains an archive resource with no checksum
- **THEN** the system SHALL report a configuration error and refuse to start

### Requirement: Fetched preset must contain the expected manifest file
After fetching a preset from any external source, the system SHALL verify that the resolved directory contains the manifest file appropriate for the preset type (infrastructure or installation) before treating the fetch as successful.

#### Scenario: Missing manifest file returns an error
- **WHEN** the fetched content does not contain the expected manifest file for the preset type
- **THEN** the system SHALL return an error naming the missing manifest and the source
