Process notes (not tasks, apply throughout):

- Each numbered item below is one commit. Commit subjects follow Conventional Commits
  (`type(scope): Subject`), properly capitalized, subject line only, no body.
- Every commit SHALL pass `task all` on its own, in isolation, at the point it lands in
  history, never relying on a later commit to make an earlier one compile, lint clean, or pass
  tests.
- Every commit adds a complete, wired, tested increment.
- OpenSpec artifacts for this change (this directory) are not committed until the change is
  archived.

## 1. Resource cache: identity and checksum handling

- [x] 1.1 Allow a statically defined `file://` (or bare local-path) artifact to omit a
  checksum, since its integrity comes from being part of the same versioned repository commit
  rather than a hand-authored value, with a test covering both the accepted no-checksum case
  and the pre-existing checksum-required case for other source types.
- [x] 1.2 Derive an `embed: true` resource's cache identity from a content hash computed at
  build time by the embedded resource generator, instead of a runtime path-based hash, with a
  test asserting two different embedded payloads for the same resource ID produce two
  different cache entries.

## 2. Glob-based resource groups

- [x] 2.1 Add a `glob: true` resource-definition flag and validate a glob template's
  `resource_path` as the required glob pattern: rejected as invalid if empty; required to pair
  with `extract: true` for a non-git, non-local source (an archive needs unpacking before it
  has a directory tree to glob within); rejected if paired with `extract: true` for a git
  source (a checkout is already a directory). Test each validation outcome.
- [x] 2.2 Add `GlobMatches(root, pattern)`, a pure, source-agnostic function that matches a
  glob pattern against an already-resolved root's own entries (files or directories alike),
  excluding a matched `.git` metadata directory. Test it against a plain directory (including a
  nested subdirectory pattern) and against a real git clone (produced directly through
  `GitSource`, the same way its own tests do) for the `.git` exclusion, plus the
  missing-pattern error case. Git-specific clone/ref mechanics otherwise stay covered by
  `GitSource`'s own direct-call tests.
- [x] 2.3 Add `Manager.RequestMember`/`RequestMemberCopy`: resolve a glob template's own
  artifact through the ordinary `Get` pipeline (respecting its declared `extract`), then match
  `GlobMatches` against the result to address one member by name — never an independent fetch,
  cache entry, or embed. `Request`/`RequestCopy` become thin wrappers calling these with an
  empty member. Add `Manager.GroupMembers`/`RegisterGroupMembers`, backed by member names the
  build-time generator registers alongside a glob template's embedded data, so listing a
  group's members never needs to extract the embedded archive or re-match live. Test resolving
  a member within a local directory and within an extracted archive, an unknown-member error,
  and `GroupMembers` against both a registered and a never-registered group.
- [x] 2.4 At generation time, for a `glob: true` resource being embedded, match its
  `resource_path` against its resolved root and embed an archive of only the matched entries —
  never the root's unfiltered content — registering the matched member names alongside the
  embedded data. A glob template resolved from embedded data is always extracted regardless of
  its own declared `extract` value, since the embedded form is always an archive even when the
  live source needs no extraction to read. Test the embedded archive excluding unmatched
  entries, and resolving a member correctly whether the live source needed extraction or not.
- [x] 2.5 Resolve the generator's relative local artifact URLs against the repository root
  rather than the invoking process's working directory, with a test invoking generation from
  two different working directories and asserting identical output.

## 3. Preset embedding migration

- [x] 3.1 Extract a dependency-free `internal/launcherpaths` package holding the path
  resolution `internal/presets` and `internal/runtimeartifacts` both need, avoiding the import
  cycle that switching preset reads onto the resource cache would otherwise create.
- [x] 3.2 Declare the built-in infrastructure and installation preset directories as
  glob-based resource groups in `resources.yaml`, with a test asserting every existing preset
  directory resolves as its own resource.
- [x] 3.3 Switch preset directory reads to the resource cache and remove the native Go source
  embedding of preset directories. `cmd/exasol` builds one process-wide `runtimeartifacts.Manager`
  and attaches it to the root `context.Context`; every preset read/write, the deployment backend,
  and every other `internal/deploy` call site that previously built its own Manager (local VM
  status/diagnostics, connect failure diagnosis) now resolve it via `runtimeartifacts.FromContext(ctx)`
  instead, and `internal/presets` no longer imports or knows about `runtimeartifacts.Cache`
  directly — it only asks the Manager to resolve a resource or list a group's members
  (`Manager.GroupMembers`). Update existing
  preset-reading tests to exercise the new path, with a `context.Context`-carried test Manager
  in place of the real default cache. This also makes the unknown-infrastructure-preset and
  unknown-installation-preset error paths independently testable for the first time, since they
  no longer depend on the real default cache to exercise; add tests for both, plus for listing a
  populated and an empty preset group.

## 4. Documentation

- [x] 4.1 Document glob-based resource groups in `doc/development.md`, alongside the existing
  embedded-resources documentation, for anyone adding a new built-in resource category.
