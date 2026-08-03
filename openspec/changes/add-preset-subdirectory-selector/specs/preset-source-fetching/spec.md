## ADDED Requirements

### Requirement: Preset resource paths select a subdirectory of the resolved source

When a preset source is fetched from a git repository or extracted from an archive, the
runtime-artifact layer SHALL support a `ResourcePath` that selects a subdirectory of the
resolved source as the returned path. `ResourcePath` SHALL be honoured for git sources even
when `Extract` is false, since a git clone already produces a browsable directory tree.

The `ResourcePath` SHALL be validated to stay within the resolved source root. Any traversal
attempt SHALL be rejected before the path is returned.

Cache identity SHALL continue to include `ResourcePath`, so two requests for the same URL
at the same content version but different subpaths SHALL be treated as distinct cache
entries.

#### Scenario: Git source honours ResourcePath as the returned path

- **WHEN** a `ResourceDefinition` with a git URL and a non-empty `ResourcePath` is resolved
- **THEN** the system SHALL clone the repository and return the path of that subdirectory
  within the clone

#### Scenario: Git source rejects a ResourcePath containing traversal

- **WHEN** the `ResourcePath` contains `..` segments that resolve outside the clone root
- **THEN** the system SHALL return an error and SHALL NOT return any resolved path

#### Scenario: Different subpaths produce distinct cache entries

- **WHEN** two requests target the same URL and revision but different `ResourcePath`
  values
- **THEN** the system SHALL treat them as distinct cache entries and resolve each subpath
  independently
