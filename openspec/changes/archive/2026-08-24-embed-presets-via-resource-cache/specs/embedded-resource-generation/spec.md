## ADDED Requirements

### Requirement: An embedded resource's cache identity comes from its build-time content
An `embed: true` resource's cache identity SHALL be derived from a content hash computed by
the build-time generator over the resource's actual embedded bytes, not from a runtime
path-based hash.

#### Scenario: Different embedded content produces different cache identity
- **WHEN** two builds embed different content under the same resource ID
- **THEN** each build's binary resolves that resource ID to its own, distinct cache entry

#### Scenario: An upgraded binary does not reuse a stale cache entry
- **WHEN** a binary embedding new content for a resource ID runs after a previous binary
  already cached that resource ID's old content
- **THEN** the new binary resolves and caches its own content rather than reusing the old
  cache entry

### Requirement: Built-in preset directories are embedded through generation
Built-in infrastructure and installation preset directories SHALL be embedded through the
build-time resource generator, not through native Go source embedding.

#### Scenario: A new preset directory needs no source-code edit
- **WHEN** a new built-in preset directory is added under the existing preset catalog entry's
  matching location
- **THEN** the build-time generator embeds it without any change to Go source code

#### Scenario: Preset content resolves through the resource cache at runtime
- **WHEN** the launcher reads a built-in preset directory at runtime
- **THEN** it resolves that directory through the resource cache, exactly as any other
  `embed: true` resource
