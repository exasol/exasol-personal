## Why

Built-in infrastructure/installation preset directories are embedded at compile time via
native `//go:embed all:infrastructure/**` / `all:installation/**` directives. Every new preset
directory therefore needs a matching source-code edit, and preset content is permanently baked
into the same binary and repository as the launcher itself.

The runtime-artifact resource cache (`internal/runtimeartifacts`) already knows how to fetch,
cache, and verify content from local directories, git repositories, and archives — the same
three shapes a preset could plausibly live in, including a future external repository. Routing
preset embedding through that cache instead removes the native-embed coupling and the
per-preset catalog edit, without changing any preset's observable behavior.

## What Changes

- A resource definition can declare `glob: true` with a glob pattern in `resource_path`: the
  entry stays one resource, resolved through the existing fetch/extract/embed pipeline, but its
  `resource_path` is matched against the resolved result's own entries (files or directories
  alike) to address a member — uniformly for a local directory, a cloned git repository, or an
  extracted archive. No per-subdirectory catalog edit is needed.
- An embedded glob template's archive contains only its matched entries, and the build-time
  generator registers the matched member names alongside the embedded data, so a running binary
  can list a group's members without extracting the embedded archive.
- Built-in infrastructure/installation preset directories are declared as glob-based resource
  groups and read through the resource cache; the native `//go:embed` of preset directories is
  removed.
- A statically defined `file://` (or bare local path) artifact may omit a checksum, since its
  integrity comes from being part of the same versioned repository commit rather than a
  hand-authored value.
- An `embed: true` resource's cache identity is derived from a content hash computed at build
  time, instead of a runtime path-based hash, so an upgraded binary's embedded content always
  resolves to its own cache entry.

## Capabilities

### New Capabilities
- `glob-based-resource-groups`: a resource-definition flag and matching glob pattern that let
  one catalog entry address a member of its own resolved content by name, uniformly across
  local, git, and archive sources, with build-time-registered member names for listing a
  group's contents without live re-matching.

### Modified Capabilities
- `runtime-artifact-cache`: a statically defined `file://`/local-path artifact may omit a
  checksum.
- `embedded-resource-generation`: an embedded resource's cache identity comes from a build-time
  content hash rather than a runtime path hash; built-in preset directories are embedded
  through generation rather than native Go source embedding.

## Impact

- `internal/runtimeartifacts`: new `Glob`/glob-pattern validation on `ResourceDefinition`, a
  source-agnostic pattern-matching primitive over an already-resolved root, and
  `Manager.RequestMember`/`RequestMemberCopy`/`GroupMembers` for resolving and listing a glob
  template's members.
- `tools/resourceembedder`: generation-time matching of a glob template's pattern against its
  resolved root, embedding only the matched entries and registering their names.
- `assets/resources/resources.yaml`: built-in preset directories declared as glob-based
  resource groups.
- `internal/presets`: preset directory reads switch from native Go embedding to the resource
  cache; a new dependency-free `internal/launcherpaths` package avoids an import cycle this
  introduces between `internal/runtimeartifacts` and `internal/presets`/`internal/config`.
- `cmd/exasol`, `internal/deploy`: one `runtimeartifacts.Manager` per process, built once and
  carried on `context.Context`, replaces each call site building its own.
- `doc/development.md`: documents glob-based resource groups for future built-in resource
  categories.
- No change to CLI behavior, preset authoring, or any existing preset's runtime behavior.
