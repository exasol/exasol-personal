# resource-cache Specification

## Purpose

Defines how launcher resources are fetched, verified, cached, inspected, and
cleaned.

## Requirements
### Requirement: Resources SHALL be cached per user
The launcher SHALL maintain a per-user cache for resources that can be reused across launcher operations.

#### Scenario: Resource is materialized on demand
- **WHEN** a launcher operation requires a resource that is not present in the user's cache
- **THEN** the launcher materializes the artifact before using it
- **AND** the artifact is recorded in the user's cache metadata

#### Scenario: Cached resource is reused
- **WHEN** a launcher operation requires a resource that is already present and valid in the user's cache
- **THEN** the launcher reuses the cached artifact
- **AND** the artifact is not downloaded again

### Requirement: Resource cache entries SHALL track last use
The launcher SHALL record when each cached resource was last used.

#### Scenario: New artifact records last use
- **WHEN** a resource is added to the user's cache
- **THEN** the cache metadata records a last-use timestamp for that artifact

#### Scenario: Cache hit updates last use
- **WHEN** a launcher operation reuses a cached resource
- **THEN** the cache metadata updates that artifact's last-use timestamp

### Requirement: Resource changes SHALL refresh cached artifacts
The launcher SHALL treat a cached resource as invalid when the artifact requested by the launcher no longer matches the artifact recorded in the cache.

#### Scenario: Changed artifact is refreshed
- **WHEN** a launcher operation requires a resource
- **AND** the user's cache contains an older or different artifact for the same runtime need
- **THEN** the launcher materializes the requested artifact before using it
- **AND** the previous cached artifact remains eligible for cleanup

### Requirement: Resource downloads SHALL be verified before use
The launcher SHALL verify downloaded resources before making them available for use.

#### Scenario: Verification succeeds
- **WHEN** a resource is downloaded
- **AND** the downloaded artifact matches the expected integrity metadata
- **THEN** the launcher may record and use the artifact

#### Scenario: Verification fails
- **WHEN** a resource is downloaded
- **AND** the downloaded artifact does not match the expected integrity metadata
- **THEN** the launcher rejects the artifact
- **AND** the rejected artifact is not recorded as usable in the cache

### Requirement: Cache cleanup SHALL use configured retention
The launcher SHALL use per-user cache configuration to determine when cached resources are old enough to be cleaned.

#### Scenario: Configured retention identifies stale artifacts
- **WHEN** cache cleanup evaluates cached resources
- **THEN** artifacts whose last-use timestamp is older than the configured retention are treated as stale
- **AND** artifacts whose last-use timestamp is within the configured retention are preserved

#### Scenario: Missing retention configuration uses a default
- **WHEN** cache cleanup runs without an existing user retention configuration
- **THEN** the launcher uses a default retention value

### Requirement: Cache cleanup SHALL remove stale artifacts
The launcher SHALL remove stale resources and their cache metadata during cleanup.

#### Scenario: Manual cleanup removes stale artifacts
- **WHEN** a user invokes cache cleanup
- **THEN** the launcher removes cached resources that are stale
- **AND** the launcher reports the cleanup result

#### Scenario: Automatic cleanup is attempted during cache use
- **WHEN** the launcher successfully uses the resource cache
- **AND** automatic cleanup is due
- **THEN** the launcher attempts to remove stale cached resources

### Requirement: Cache cleanup SHALL support corrupted artifact removal
The launcher SHALL provide a way to remove cached resources that fail integrity verification.

#### Scenario: User cleans corrupted artifacts
- **WHEN** a user invokes cache cleanup for corrupted resources
- **THEN** the launcher removes cached resources that fail integrity verification
- **AND** the launcher reports the cleanup result

### Requirement: Cache cleanup SHALL support full cache removal
The launcher SHALL provide a way to remove all cached resources.

#### Scenario: User removes all cached artifacts
- **WHEN** a user invokes cleanup for all cached resources
- **THEN** the launcher removes all cached resources
- **AND** the launcher reports the cleanup result

#### Scenario: Full cleanup removes unindexed cache contents
- **WHEN** a user invokes cleanup for all cached resources
- **AND** the resource cache contains files or directories not referenced by cache metadata
- **THEN** the launcher removes those unreferenced cache contents
- **AND** the launcher resets resource cache metadata

### Requirement: Cache cleanup SHALL support partial download removal
The launcher SHALL provide a way to remove resource materializations that
were interrupted before they were committed as usable cached artifacts.

#### Scenario: User cleans incomplete entries
- **WHEN** a user invokes cache cleanup for incomplete entries
- **THEN** the launcher removes resource materializations that did not
  complete
- **AND** indexed cached resources remain available
- **AND** the launcher reports the cleanup result

### Requirement: Cache cleanup SHALL support previewing cleanup
The launcher SHALL provide a way to preview selected cleanup work without changing cache contents or cache metadata.

#### Scenario: User previews indexed cleanup
- **WHEN** a user invokes cache cleanup in preview mode for cleanup that selects cached resources
- **THEN** the launcher reports which indexed cached resources would be removed
- **AND** cached resources are not removed
- **AND** cache metadata is not changed

#### Scenario: User previews incomplete entry cleanup
- **WHEN** a user invokes cache cleanup for incomplete entries in preview mode
- **THEN** the launcher reports which incomplete resource
  materializations would be removed
- **AND** those materializations are not removed
- **AND** cache metadata is not changed

### Requirement: Cache listing SHALL report cached artifacts
The launcher SHALL provide a way to list cached resources.

#### Scenario: User lists cached artifacts
- **WHEN** a user requests the resource cache contents
- **THEN** the launcher reports each cached resource
- **AND** the report includes each artifact's creation and last-use timestamps,
  source URL, and identity

#### Scenario: Empty cache is listed
- **WHEN** a user requests the resource cache contents
- **AND** no resources are cached
- **THEN** the launcher reports that the cache is empty

### Requirement: Cache unlocking SHALL support stale-lock recovery

The launcher SHALL provide a way to clear a stale resource cache lock.

#### Scenario: User clears stale cache lock

- **WHEN** a user requests cache unlocking
- **THEN** the launcher clears the cache lock if one exists
- **AND** the launcher reports the unlock result as an operational notice on standard error

### Requirement: Cache diagnostics SHALL report cache state without mutation
The launcher SHALL provide resource cache diagnostics that inspect cache state without changing cache contents or cache locks.

#### Scenario: User inspects cache diagnostics
- **WHEN** a user requests resource cache diagnostics
- **THEN** the launcher reports cache state information
- **AND** cached resources are not removed
- **AND** cache locks are not cleared

#### Scenario: Diagnostics report corrupted artifacts
- **WHEN** a user requests resource cache diagnostics
- **AND** one or more cached resources fail integrity verification
- **THEN** cache diagnostics report those artifacts as corrupted
- **AND** cached resources are not removed

#### Scenario: Diagnostics report cache problems
- **WHEN** resource cache metadata, configuration, integrity, or lock state cannot be interpreted normally
- **THEN** cache diagnostics report the observed problem
- **AND** diagnostics continue reporting any remaining cache state that can be inspected

### Requirement: Resource cache operations SHALL coordinate concurrent access
The launcher SHALL coordinate operations that read or mutate resource cache state so concurrent launcher processes do not corrupt cached artifacts or cache metadata.

#### Scenario: Concurrent cache mutation is serialized
- **WHEN** multiple launcher processes attempt to mutate resource cache state concurrently
- **THEN** only one mutation proceeds at a time

#### Scenario: Cache operation reports lock contention
- **WHEN** a cache operation cannot proceed because another process holds the cache
- **THEN** the launcher reports that the cache is currently locked

### Requirement: Resource cache SHALL support fetching from git repository sources
The resource cache SHALL accept git repository URLs (`git@`, `git://`,
`https://*.git`, `http://*.git`) as artifact sources, cloning or updating the
repository into the cache. A resource specification SHALL declare the branch,
tag, or commit to check out as the artifact's `ref`.

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

#### Scenario: Commit SHA is resolved without contacting the remote
- **WHEN** a git repository source is requested with a full 40-character commit
  SHA
- **AND** a cache entry for that commit already exists
- **THEN** the cache SHALL return the cached path without contacting the remote

### Requirement: Resource cache SHALL identify git artifacts by resolved commit hash
The resource cache SHALL use the commit hash resolved before fetching as the content identity for git sources, not a user-supplied checksum.

#### Scenario: Git artifact cache key uses commit hash
- **WHEN** a git artifact is stored in the cache
- **THEN** the cache entry records the resolved commit hash as the content identity

#### Scenario: Git artifact definitions SHALL NOT specify a checksum
- **WHEN** a statically defined git artifact specifies a checksum
- **THEN** the cache SHALL reject the definition with a configuration error

### Requirement: Resource cache SHALL support fetching from local filesystem sources
The resource cache SHALL accept `file://` URIs pointing to local directories or archive files as artifact sources. A local directory SHALL occupy no cache entry, since the cache neither stores nor manages content it did not fetch.

#### Scenario: file:// directory source is returned directly
- **WHEN** a `file://` URI points to an existing local directory
- **THEN** the cache SHALL return the resolved absolute path directly without copying, extracting, or creating a symlink
- **AND** the cache SHALL record no cache entry for it

#### Scenario: file:// archive source is extracted into the cache
- **WHEN** a `file://` URI points to a local archive file in a supported format
- **THEN** the cache SHALL extract the archive into a cache entry

#### Scenario: Changed local archive is extracted again
- **WHEN** a `file://` URI points to a local archive that has changed since it
  was last extracted
- **THEN** the cache SHALL extract the changed archive rather than reusing the
  previous extraction

#### Scenario: file:// path that does not exist returns an error
- **WHEN** a `file://` URI points to a path that does not exist
- **THEN** the cache SHALL return an error that includes the path

### Requirement: Resource cache SHALL support ZIP archive extraction
The resource cache SHALL extract `.zip` archives in addition to `.tar.gz`/`.tgz` archives when materialising archive-type artifacts.

#### Scenario: .zip archive artifact is extracted
- **WHEN** an artifact source resolves to a `.zip` archive
- **THEN** the cache SHALL extract the archive contents into the cache entry

#### Scenario: Unsupported archive format returns an error
- **WHEN** an artifact source resolves to a file with an unrecognised archive format
- **THEN** the cache SHALL return an error identifying the unsupported format

### Requirement: Resource cache SHALL extract directory entries within a tar.gz archive
The resource cache SHALL extract an explicit directory entry within a
tar.gz archive as a directory, in addition to the regular file and symlink
entries it already extracts.

#### Scenario: Directory entry is created
- **WHEN** a tar.gz archive being extracted contains an explicit directory
  entry
- **THEN** the cache SHALL create that directory in the extraction target
- **AND** extraction of the remaining entries in the archive SHALL continue

#### Scenario: Entries nested under an extracted directory entry are extracted
- **WHEN** a tar.gz archive contains a directory entry followed by regular
  file or symlink entries nested under that directory
- **THEN** the cache SHALL extract all of those nested entries into the
  created directory

### Requirement: Resource cache SHALL support platform-independent artifact definitions
The resource cache SHALL accept artifact definitions that use an `"any"` platform key, resolving that definition for any platform when no platform-specific variant is present.

#### Scenario: Platform-independent artifact is resolved for any platform
- **WHEN** an artifact definition specifies only an `"any"` platform key
- **THEN** the cache SHALL resolve that definition regardless of the current platform

#### Scenario: Platform-specific artifact takes precedence over platform-independent
- **WHEN** an artifact definition contains both a platform-specific key and an `"any"` key
- **THEN** the cache SHALL resolve the platform-specific variant for a matching platform

### Requirement: Resource cache SHALL always re-fetch archive artifacts without a checksum
The resource cache SHALL re-fetch an archive artifact it cannot identify on every request, replacing any existing cache entry.
An archive artifact declaring no checksum whose server offers no strong validator
cannot be reliably cached by content identity. Where a server does offer a strong
validator for such an artifact, the cache SHALL identify the artifact by its
source location and validator, and reuse the cached copy while it is unchanged.

#### Scenario: Unidentifiable archive is re-fetched on every request
- **WHEN** an archive artifact with no checksum is requested
- **AND** its server offers no strong validator for it
- **THEN** the cache SHALL re-fetch the artifact regardless of any existing cache entry

#### Scenario: Re-fetch of an unidentifiable archive is logged
- **WHEN** an archive artifact that cannot be identified is re-fetched
- **THEN** the cache SHALL emit a log message indicating the source is being re-fetched because it could not be identified

#### Scenario: Strong validator identifies a checksumless archive
- **WHEN** an archive artifact with no checksum is requested
- **AND** its server offers a strong validator for it
- **AND** that validator matches an existing cache entry
- **THEN** the cache SHALL return the cached artifact without re-fetching it

#### Scenario: Weak validator does not identify an archive
- **WHEN** an archive artifact with no checksum is requested
- **AND** its server offers only a weak validator for it
- **THEN** the cache SHALL re-fetch the artifact regardless of any existing cache entry

#### Scenario: Equal validators from different locations remain distinct
- **WHEN** two archive locations offer the same strong validator
- **THEN** the cache SHALL store and resolve each location's content separately

### Requirement: Resource cache SHALL support runtime-constructed artifact definitions
The resource cache SHALL resolve artifact definitions that are constructed at runtime by callers, without requiring those definitions to be registered in the static resource catalog.

#### Scenario: Runtime-constructed definition is resolved
- **WHEN** a caller supplies an artifact definition directly at resolution time
- **THEN** the cache SHALL resolve and cache the artifact using that definition

#### Scenario: Runtime-constructed definition may omit a checksum
- **WHEN** a runtime-constructed archive artifact definition does not specify a checksum
- **THEN** the cache SHALL resolve the artifact applying the no-checksum re-fetch policy

### Requirement: A statically defined local-path artifact may omit a checksum
A statically defined `file://` or bare local-path artifact SHALL NOT be required to declare a checksum, since its integrity comes from being part of the same versioned repository commit as the launcher itself, not from a hand-authored value. A container image artifact whose reference names its content by digest SHALL NOT be required to declare one either, since the digest already is that checksum.

#### Scenario: Local-path artifact without a checksum is accepted
- **WHEN** a statically defined resource declares a `file://` or bare local-path artifact with
  no checksum
- **THEN** the specification is accepted

#### Scenario: Digest-pinned container image without a checksum is accepted
- **WHEN** a statically defined resource declares a container image artifact whose
  reference names a digest, and specifies no checksum
- **THEN** the specification is accepted

#### Scenario: Container image without a digest still requires a checksum
- **WHEN** a statically defined resource declares a container image artifact whose
  reference names no digest, and specifies no checksum
- **THEN** the specification is rejected as invalid

#### Scenario: Non-local artifact still requires a checksum
- **WHEN** a statically defined resource declares an artifact whose source is neither a git
  repository, a local path, nor a digest-pinned container image, and specifies no checksum
- **THEN** the specification is rejected as invalid, exactly as before this capability existed

### Requirement: An embedded resource source materializes resources from data compiled into the binary
The resource specification SHALL address data compiled into the binary through an
`embedded://` source, resolved from that data alone and never from a network
source. A resource whose source is not `embedded://` SHALL resolve only from its
own source, and SHALL never consult embedded data.

#### Scenario: Embedded data resolves a resource without network access
- **WHEN** a resource declares an `embedded://` source and matching data is present in the binary
- **THEN** the resource is materialized from that embedded data without contacting any network source

#### Scenario: Missing embedded data is a hard failure, not a fallback
- **WHEN** a resource declares an `embedded://` source but no matching data is present in the binary
- **THEN** resolving that resource fails, and no network source is attempted

#### Scenario: A resource with another source never consults embedded data
- **WHEN** a resource declares a source other than `embedded://`
- **THEN** it resolves only from that source, regardless of whether the binary contains embedded data under the same resource identifier

#### Scenario: Embedded resource extraction reuses existing extraction
- **WHEN** a resource with an `embedded://` source and archive extraction enabled is materialized
- **THEN** it is extracted using the same extraction mechanism used for a network-fetched archive of that format

#### Scenario: Embedded resource content distinguishes launcher versions
- **WHEN** two launcher versions embed different content under the same resource identifier
- **THEN** each resolves to its own cached artifact

### Requirement: A cached resource SHALL be identified by what its source can state before transfer
The resource cache SHALL identify a cached artifact by an identity its
source can state before transferring content, and SHALL reuse a cached artifact
only when that identity matches. When a source cannot state an identity, the
cache SHALL re-fetch the artifact on every request.

#### Scenario: Declared checksum identifies an artifact
- **WHEN** an artifact declares a checksum
- **THEN** the cache identifies the artifact by that checksum

#### Scenario: Source-stated identity is used when no checksum is declared
- **WHEN** an artifact declares no checksum
- **AND** its source can state an identity for the content before transferring it
- **THEN** the cache identifies the artifact by that identity
- **AND** a subsequent request with the same identity reuses the cached artifact
  without re-fetching

#### Scenario: Identity resolution failure is reported
- **WHEN** a source cannot be reached to state an identity for an artifact
- **THEN** the launcher reports that failure

### Requirement: A cache from a different launcher version SHALL be handled without blocking the user
The launcher SHALL continue to operate when the resource cache was written by a different launcher version. A cache written by an earlier version SHALL be replaced rather than reused, and the launcher SHALL tell the user how to reclaim the space its contents still occupy. A cache written by a later version SHALL be reported with the action that resolves it.

#### Scenario: Cache from an earlier launcher version does not block operation
- **WHEN** the resource cache was written by an earlier launcher version
- **THEN** the launcher completes the requested operation
- **AND** artifacts recorded by that earlier version are not reused

#### Scenario: Superseded cache contents are reported as reclaimable
- **WHEN** the resource cache was written by an earlier launcher version
- **AND** the user runs a command producing text output
- **THEN** the launcher reports that those contents are no longer used and names
  the command that reclaims their space

#### Scenario: Cache from a later launcher version is reported with its remedy
- **WHEN** the resource cache was written by a later launcher version
- **THEN** the launcher reports that the cache came from a newer version
- **AND** the report names the actions that resolve it

### Requirement: A cached resource SHALL become available only when it is complete
The resource cache SHALL make a cached artifact available only after it
has been fetched, verified, and unpacked in full. An operation interrupted before
that point SHALL leave no cached artifact that a later operation treats as
usable.

#### Scenario: Interrupted materialization leaves no usable artifact
- **WHEN** materializing a resource is interrupted before it completes
- **THEN** the cache does not report the artifact as cached
- **AND** a later request for that artifact materializes it again

#### Scenario: Interrupted materialization leaves reclaimable space
- **WHEN** materializing a resource is interrupted before it completes
- **THEN** a user invoking cleanup for incomplete entries reclaims the space it
  occupied

### Requirement: Selecting a subpath or extracting an artifact SHALL NOT create a separate cached copy
The resource cache SHALL treat a subpath selection and an extraction
choice as ways of presenting one cached artifact, not as part of its identity. Two
requests differing only in the subpath they select, or only in whether they
extract, SHALL share one cached copy of the fetched content.

#### Scenario: Two subpaths of one source share a cached copy
- **WHEN** a source is requested twice, selecting a different subpath each time
- **THEN** the cache fetches the source once
- **AND** each request resolves to its own selected subpath

#### Scenario: Extracted and unextracted views share a cached download
- **WHEN** one artifact is requested with extraction and another with the same
  source and no extraction
- **THEN** the cache downloads the source once
- **AND** both requests resolve to content in the requested form

### Requirement: A container image artifact SHALL be identified by its registry digest
The resource cache SHALL identify a container image artifact by the
digest the registry reports for it, and SHALL NOT require the stored image
archive to be byte-identical across platforms.

#### Scenario: Registry digest identifies a container image
- **WHEN** a container image artifact is requested
- **THEN** the cache identifies it by the digest reported by the registry

#### Scenario: Same image is reused across requests
- **WHEN** a container image artifact is requested a second time
- **AND** the registry reports the same digest
- **THEN** the cache returns the stored image archive without transferring it
  again
