## ADDED Requirements

### Requirement: Plain preset name resolves to an embedded preset
A preset argument that is a plain identifier (no URI scheme, no filesystem path separators) SHALL resolve exclusively against the binary's embedded asset catalog and SHALL NOT trigger any external fetch.

#### Scenario: Known embedded name uses embedded asset
- **WHEN** the user passes a plain name that matches an embedded preset
- **THEN** the system SHALL use the embedded asset

#### Scenario: Unknown plain name returns a descriptive error
- **WHEN** the user passes a plain name that does not match any embedded preset
- **THEN** the system SHALL return an error that includes the unknown name and the list of available embedded preset names

### Requirement: URI-scheme arguments identify external preset sources
A preset argument that begins with a recognised URI scheme (`file://`, `https://`, `http://`, `git://`, or `git@`) SHALL be resolved as an external source and SHALL NOT be matched against the embedded asset catalog, regardless of what the path component contains.

#### Scenario: https:// ending in .git resolves as a git repository
- **WHEN** the user passes an `https://` URL whose path ends with `.git`
- **THEN** the system SHALL resolve the preset by fetching the git repository at that URL

#### Scenario: git:// URL resolves as a git repository
- **WHEN** the user passes a `git://` URL
- **THEN** the system SHALL resolve the preset by fetching the git repository at that URL

#### Scenario: git@ URL resolves as a git repository
- **WHEN** the user passes a `git@` URL
- **THEN** the system SHALL resolve the preset by fetching the git repository at that URL

#### Scenario: http:// URL ending in .git resolves as a git repository
- **WHEN** the user passes an `http://` URL whose path ends with `.git`
- **THEN** the system SHALL resolve the preset by fetching the git repository at that URL

#### Scenario: file:// URI pointing to a directory resolves directly
- **WHEN** the user passes a `file://` URI whose path is a directory
- **THEN** the system SHALL resolve the preset from that directory without copying or caching it

#### Scenario: file:// URI pointing to a supported archive is extracted
- **WHEN** the user passes a `file://` URI whose path is a `.tar.gz`, `.tgz`, or `.zip` file
- **THEN** the system SHALL extract the archive into the local cache and resolve the preset from the extracted contents

#### Scenario: file:// URI pointing to an unsupported file is rejected
- **WHEN** the user passes a `file://` URI whose path is neither a directory nor a supported archive
- **THEN** the system SHALL return an error stating that the path must be a directory or a supported archive file

#### Scenario: https:// URL with a recognised archive extension resolves as a remote archive
- **WHEN** the user passes an `https://` URL whose path ends with a recognised archive extension (`.tar.gz`, `.tgz`, or `.zip`)
- **THEN** the system SHALL resolve the preset by downloading and extracting the archive at that URL

#### Scenario: http:// URL with a recognised archive extension resolves as a remote archive
- **WHEN** the user passes an `http://` URL whose path ends with a recognised archive extension (`.tar.gz`, `.tgz`, or `.zip`)
- **THEN** the system SHALL resolve the preset by downloading and extracting the archive at that URL

#### Scenario: URI scheme takes precedence over embedded catalog
- **WHEN** the user passes a URI whose final path component matches an embedded preset name
- **THEN** the system SHALL use the external source and SHALL NOT use the embedded asset

#### Scenario: @ref suffix on a non-git source is rejected
- **WHEN** the user passes a scheme-based URI that contains a `@`-separated suffix, and the URL before that suffix does not end in `.git` and does not use the `git://` scheme
- **THEN** the system SHALL return an error stating that the `@ref` syntax is only supported for git source URLs

### Requirement: Git repository URLs support an optional revision specifier
A git repository URL MAY include a revision specifier appended as `@<ref>`, where `<ref>` is a branch name, tag name, or full commit SHA. When a specifier is present, the system SHALL fetch the repository at that revision. When absent, the system SHALL fetch the repository's default branch HEAD.

#### Scenario: URL with branch name fetches that branch
- **WHEN** the user passes a git URL with an `@<branch>` suffix
- **THEN** the system SHALL resolve the preset from the tip of that branch

#### Scenario: URL with tag fetches that tag
- **WHEN** the user passes a git URL with an `@<tag>` suffix
- **THEN** the system SHALL resolve the preset at that tag

#### Scenario: URL with commit SHA fetches that exact commit
- **WHEN** the user passes a git URL with an `@<commit-sha>` suffix
- **THEN** the system SHALL resolve the preset at that exact commit

#### Scenario: URL without revision specifier fetches default branch HEAD
- **WHEN** the user passes a git URL with no `@<ref>` suffix
- **THEN** the system SHALL resolve the preset from the repository's default branch HEAD

### Requirement: Filesystem path arguments resolve from the local filesystem
A preset argument that resembles a filesystem path (contains a path separator, or begins with `.` or `~`) SHALL be resolved from the local filesystem.

#### Scenario: Relative path resolves from local filesystem
- **WHEN** the user passes `./my-preset`
- **THEN** the system SHALL resolve the preset from that local directory

#### Scenario: Windows relative path resolves from local filesystem
- **WHEN** the user passes `.\my-preset`
- **THEN** the system SHALL resolve the preset from that local directory

#### Scenario: Absolute path resolves from local filesystem
- **WHEN** the user passes `/opt/presets/custom`
- **THEN** the system SHALL resolve the preset from that local directory
