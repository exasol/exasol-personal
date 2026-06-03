# runtime-resource-management Specification

## Purpose

Define how Exasol Personal manages runtime resources in the deployment directory through resource definition files and deployment-local caching.

## ADDED Requirements

### Requirement: Runtime resources SHALL be made available on demand

The launcher SHALL use a resource definition file to make each required runtime resource available when needed.

#### Scenario: Launcher gets a required resource
- **WHEN** the launcher needs a runtime resource for a command or workflow
- **THEN** the resource is made available for use
- **AND** it does not need to know the artifact URL, checksum, extraction behavior, or platform mapping

#### Scenario: Cached resource path is reused
- **WHEN** a runtime resource already exists in the local cache and matches the current definition for that resource
- **THEN** the cached resource is reused
- **AND** the artifact is not downloaded again

### Requirement: Runtime resources SHALL be cached in the deployment directory

The application SHALL store downloaded artifacts and cache metadata inside the deployment directory.

#### Scenario: Cache metadata is written in the deployment directory
- **WHEN** the application materializes a resource
- **THEN** it records the resource in deployment-local cache metadata
- **AND** the cache metadata is sufficient to determine whether the resource is already present and valid

#### Scenario: Cache metadata records the source URL
- **WHEN** the application writes or updates the cache metadata
- **THEN** it records the URL used to obtain each cached resource

#### Scenario: Missing resource is downloaded into the deployment directory
- **WHEN** a runtime resource is absent from the local cache
- **THEN** the application downloads it
- **AND** stores it in the deployment directory
- **AND** updates the cache metadata

#### Scenario: Resource artifacts are scoped by resource name
- **WHEN** the application stores a resource artifact
- **THEN** it stores the artifact in the deployment-local cache for that resource name
- **AND** the artifact path is isolated from other resources

### Requirement: Platform-specific artifacts SHALL be selected by current runtime platform

The application SHALL resolve platform-specific artifacts using the current runtime platform.

#### Scenario: Production uses the current platform
- **WHEN** the application resolves a platform-specific resource
- **THEN** it uses the current runtime platform

### Requirement: URL or checksum changes SHALL invalidate cached resources

The application SHALL consider a cached resource stale when either the URL or the checksum used to identify it differs from the current resource definition.

#### Scenario: Changed URL refreshes a cached resource
- **WHEN** the URL for a resource changes
- **AND** the deployment directory still contains a cached artifact for that resource
- **THEN** the cached artifact is treated as stale
- **AND** the resource is refreshed before its path is returned

#### Scenario: Changed checksum refreshes a cached resource
- **WHEN** the checksum for a resource changes
- **AND** the deployment directory still contains a cached artifact for that resource
- **THEN** the cached artifact is treated as stale
- **AND** the resource is refreshed before its path is returned

### Requirement: Resource definitions SHALL support platform-specific artifacts and paths

Each resource in a resource definition file, when present, SHALL define platform-specific download information. Optional archive extraction defaults to not extracting when omitted. A platform-specific artifact MAY define a download path override for the downloaded file. A platform-specific artifact MAY also define a path within the extracted archive that is returned to the caller when extraction is enabled. An empty resource definition file is valid and means the launcher has no runtime resources to resolve.

#### Scenario: Platform-specific artifact resource is valid
- **WHEN** a resource defines one or more platform-specific artifact entries for the current platform
- **THEN** the matching entry for the current platform is used

#### Scenario: Download path overrides the default artifact name
- **WHEN** a platform-specific artifact definition provides a download path
- **THEN** the application stores the downloaded file using that path within the resource cache

#### Scenario: Archive path is returned from the extracted archive
- **WHEN** a resource requests archive extraction
- **AND** the platform-specific artifact definition provides a path inside the archive
- **THEN** the application returns the extracted path for that archive entry to the caller

#### Scenario: Path requires archive extraction
- **WHEN** a resource definition provides a path inside the extracted archive
- **AND** the resource does not request archive extraction
- **THEN** parsing or validation fails
- **AND** the error explains that archive paths require archive extraction

### Requirement: Downloaded artifacts SHALL be verified before use

The application SHALL verify a downloaded artifact only after it has been downloaded, using the expected checksum before it is used or extracted.

#### Scenario: Checksum mismatch is reported clearly
- **WHEN** the downloaded artifact checksum does not match the expected value for the resource
- **THEN** the request fails
- **AND** the error includes the expected value
- **AND** the error includes the actual value
- **AND** the error identifies the resource being resolved

#### Scenario: Valid checksum permits use of the cached artifact
- **WHEN** the downloaded artifact checksum matches the expected value for the resource
- **THEN** the artifact is accepted for caching or extraction

### Requirement: Archive resources SHALL be optionally extracted

The application SHALL support optional archive extraction when a resource requests it. Only `.tar.gz` and `.tgz` archives are supported.

#### Scenario: Archive resource is extracted after checksum verification
- **WHEN** a resource definition requests archive extraction
- **AND** the downloaded artifact checksum matches
- **THEN** the archive is extracted for that resource
- **AND** the extracted path for the requested archive entry, when present, is returned to the caller

#### Scenario: Non-archive resource is returned without extraction
- **WHEN** a resource definition does not request archive extraction
- **THEN** the downloaded artifact path is returned directly

#### Scenario: Extraction directory is derived from the archive name
- **WHEN** an archive artifact is extracted
- **THEN** the extracted directory name is derived from the archive name
- **AND** no explicit output-name is required

#### Scenario: Unsupported archive formats are rejected
- **WHEN** a resource definition requests extraction of an artifact that is not `.tar.gz` or `.tgz`
- **THEN** the request fails
- **AND** the error explains that only `.tar.gz` and `.tgz` archives are supported
