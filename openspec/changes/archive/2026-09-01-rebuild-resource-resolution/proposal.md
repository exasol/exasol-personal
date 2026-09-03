## Why

The runtime resource system grew one use case at a time, and its abstractions no
longer describe what it does.

`resource_path` means three different things depending on which other fields are
set: a literal subpath inside an archive, a glob pattern, or a subpath of a
redirect target. `Source.Fetch` returns a redirect path instructing the caller to
ignore the destination it just passed in, and that one source's peculiarity has
become a field on the shared cache index plus branches in three unrelated places.
Content identity is smuggled through a field named `Sha256`, so a local path's
identity is a hash of its path rather than its content, and a git source's real
identity has nowhere to live. The build-time generator has to mutate resource
definitions before handing them back to the runtime, clearing `embed`, clearing
`resource_path`, and forcing `extract`, because build time and runtime need
different things from a single declaration. A glob group resolves its members by
re-matching at runtime but lists them from a separate build-time registry, so the
two halves of one concept disagree about where truth lives.

Every one of these is the same underlying problem: decisions that could be
settled once, ahead of time, are instead re-derived at each use, and the
conditionals needed to re-derive them have accumulated into the abstraction.

This change resolves those decisions ahead of time. The build produces a fully
concrete resource specification for its target platform, containing no
conditionals and no build directives, and the runtime becomes a resolver that
reads it. Sources gain a single, honest contract for what they know before
transferring content. Presets gain the ability to declare resources of their own.

## What Changes

- The runtime resource cache stores each resource in a single flat directory
  named by its identity. Entries created by earlier launcher versions are not
  reused; a new cache directory is populated on first use, and the launcher
  reports that the superseded contents can be reclaimed. A cache created by a
  later launcher version is reported together with the actions that resolve it.
- A cache entry appears only once it is complete. All sources fetch, verify, and
  extract into a staging area, and the finished entry is moved into place in one
  step, so an interrupted operation never leaves an entry that looks usable.
- `exasol cache clean --partial-downloads` is renamed to
  `exasol cache clean --incomplete`, and it removes interrupted entries, which
  the previous flag did not.
- `exasol cache list` and `exasol diag cache` report the resources sharing each
  cache entry rather than a single resource and platform, and report one entry
  path in place of separate artifact and resolved paths. Under `--json` the
  `resourceId`, `platform`, `artifactPath`, and `resolvedPath` fields are
  replaced by `resourceIds`, `identity`, and `path`.
- A cached resource is identified by whatever its source can state about it
  before transfer: the resolved commit for a git repository, the registry digest
  for a container image, a declared checksum for a downloaded archive, or a
  strong entity tag for a download with no checksum. A download whose server
  offers only a weak entity tag continues to be re-fetched on every use.
- A resource selected from a subdirectory of a git repository or archive shares
  one cached copy with every other subdirectory selection of the same source,
  instead of fetching a separate copy per subdirectory.
- A local directory used as a resource occupies no cache entry.
- Built-in preset directories are cached and materialized per preset, so
  requesting one preset no longer materializes every preset in its group.
- An infrastructure or installation preset may ship a resource specification of
  its own. Its resources are available while that preset is being evaluated, may
  override resources of the same name declared by the launcher, and are no longer
  available once evaluation finishes. Both presets are selected through launcher
  resources before either scoped layer applies. Relative locations in a preset
  specification resolve against the preset's own directory.
- A resource specification declares a source revision as `ref`, a subpath as
  `subpath`, and a build-time expansion pattern as `glob`. A container image
  resource stored in the cache is no longer rewritten to be byte-identical across
  platforms.
- A preset argument is matched against known preset names before being treated as
  a location, and an argument that is neither reports whichever of the two
  failures fits what the user wrote.
- Shared deployment assets are distributed through the resource system rather
  than a separate embedding mechanism.

## Capabilities

### New Capabilities

- `preset-resource-layering`: a preset may declare its own resource
  specification, scoped to that preset's evaluation, overriding launcher
  resources of the same name and resolving relative locations against the
  preset's own directory.

### Modified Capabilities

- `resource-cache`: identity is supplied by the source rather than
  derived from a checksum field; cache entries are stored flat by identity and
  appear only when complete; subpath and extraction selection no longer split one
  source into multiple cached copies; local directories are not cached; the
  interrupted-entry cleanup mode is renamed and made effective.
- `embedded-resource-generation`: the build emits a fully concrete resource
  specification for its target platform alongside the embedded data, so no
  embedding directive reaches the runtime; embedded data is addressed as an
  ordinary resource source.
- `glob-based-resource-groups`: a glob pattern expands at build time into one
  independently addressable resource per match, and a group's members are listed
  from the specification rather than a separate registry.
- `preset-source-fetching`: subpath selection applies uniformly to any source
  that resolves to a directory; revision and subpath are declared as named fields
  in a specification.
- `external-preset-resolution`: preset names are resolved before locations, and
  failure messages distinguish an unknown preset name from an unreachable
  location.

## Impact

- `internal/runtimeartifacts` is replaced by `internal/resource`, and
  `assets/resources` is renamed to `assets/resourcedata`. Roughly thirty
  importing files are updated.
- The resolver's public surface reduces from seven request methods and seven
  constructors to resolution, listing, and layering.
- `assets/resourcedata/resources.yaml` adopts the `ref`, `subpath`, and pattern
  valued `glob` fields.
- `tools/resourceembedder` emits data only: a resolved specification and a blob
  directory per platform. It writes no Go, so its template and formatting pass
  are removed along with the registration hooks.
- `assets/resourcedata/embedded` (new): one hand-written, build-tagged wrapper
  per supported platform exposing that platform's blobs and resolved
  specification, replacing the generated registration files. The composition
  root passes them to the resolver, so no package depends on embedded data
  through a blank import.
- `internal/presets` reads shared assets through the resource system, and its
  preset reference type collapses to a single resolution path.
- `cmd/exasol`: `cache clean --partial-downloads` becomes `--incomplete`.
- `CHANGELOG.md`: entries for the renamed flag and the cache rebuild.
