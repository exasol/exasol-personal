## ADDED Requirements

### Requirement: A subpath SHALL be selectable from any source that resolves to a directory
The system SHALL apply a subpath selection uniformly to any preset source whose resolved content is a directory, whether that directory is a cloned git repository, an extracted archive, or a local directory.

#### Scenario: Subpath selects within a cloned repository
- **WHEN** a preset source identifies a git repository and selects a subpath
- **THEN** the system resolves the preset to that subdirectory of the clone

#### Scenario: Subpath selects within an extracted archive
- **WHEN** a preset source identifies an archive and selects a subpath
- **THEN** the system resolves the preset to that subdirectory of the extracted
  content

#### Scenario: Subpath selects within a local directory
- **WHEN** a preset source identifies a local directory and selects a subpath
- **THEN** the system resolves the preset to that subdirectory

#### Scenario: Subpath outside the resolved content is rejected
- **WHEN** a preset source selects a subpath that escapes its resolved content
- **THEN** the system rejects the source with an error

### Requirement: A resource specification SHALL declare a source revision and subpath as named fields
A resource specification SHALL declare the revision to fetch as `ref` and the path to select within resolved content as `subpath`. The `@ref` and `#subpath` suffixes SHALL remain available on a preset source given as a single command-line argument.

#### Scenario: Specification declares ref and subpath as fields
- **WHEN** a resource specification declares a git source with a `ref` and a
  `subpath`
- **THEN** the system fetches that revision and resolves to that subdirectory

#### Scenario: Command-line argument accepts the suffix form
- **WHEN** a user supplies a preset source with an `@ref` suffix, a `#subpath`
  suffix, or both
- **THEN** the system resolves it identically to a specification declaring the
  same revision and subpath

## MODIFIED Requirements

### Requirement: Remote archive preset sources without a checksum are always re-fetched
The system SHALL re-download a remote archive preset source it cannot identify on every invocation, and SHALL log a message indicating that the source is being re-fetched because it could not be identified. A remote archive source that specifies no checksum cannot be cached reliably unless its server offers a strong validator for it, in which case the system SHALL identify the source by that validator and reuse the cached copy while it is unchanged.

#### Scenario: Unidentifiable archive source is re-downloaded every time
- **WHEN** the user specifies a remote archive preset source with no checksum
- **AND** its server offers no strong validator for it
- **THEN** the system SHALL download the archive on each invocation and SHALL log a message stating the reason

#### Scenario: Re-fetch of an unidentifiable archive replaces any prior cache entry
- **WHEN** an unidentifiable archive source is re-downloaded
- **THEN** the system SHALL replace any existing cache entry for that URL with the newly downloaded content

#### Scenario: Strong validator lets a checksumless archive be reused
- **WHEN** the user specifies a remote archive preset source with no checksum
- **AND** its server offers a strong validator matching a cached copy
- **THEN** the system SHALL use the cached copy without re-downloading it

### Requirement: Local file:// preset sources are handled according to their content kind
A `file://` URI pointing to a directory SHALL be used as-is without copying, extracting, or caching, and SHALL occupy no cache entry. A `file://` URI pointing to a supported archive file SHALL be extracted into the cache in the same way as a remote archive, and SHALL be extracted again when the archive changes. A `file://` URI that is neither a directory nor a supported archive SHALL be rejected with an error.

#### Scenario: file:// directory is used directly without caching
- **WHEN** the user specifies a `file://` URI pointing to a local directory
- **THEN** the system SHALL use that directory as the preset without any caching or copying
- **AND** the cache SHALL record no entry for it

#### Scenario: file:// archive is extracted into the cache
- **WHEN** the user specifies a `file://` URI pointing to a supported local archive file
- **THEN** the system SHALL extract the archive into the local cache and use the extracted contents as the preset

#### Scenario: Changed file:// archive is extracted again
- **WHEN** the user specifies a `file://` URI pointing to a local archive that has
  changed since it was last extracted
- **THEN** the system SHALL extract the changed archive rather than reusing the
  previous extraction
