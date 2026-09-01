## ADDED Requirements

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

## MODIFIED Requirements

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
- **THEN** the launcher reports an empty cache without error

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

### Requirement: Resource cache SHALL always re-fetch archive artifacts without a checksum
The resource cache SHALL re-fetch an archive artifact it cannot identify on every request, replacing any existing cache entry.
An archive artifact declaring no checksum whose server offers no strong validator
cannot be reliably cached by content identity. Where a server does offer a strong
validator for such an artifact, the cache SHALL identify the artifact by it and
reuse the cached copy while it is unchanged.

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
