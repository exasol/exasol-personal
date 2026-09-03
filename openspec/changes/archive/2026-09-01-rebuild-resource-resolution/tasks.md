Process notes (not tasks, apply throughout):

- Each numbered item below is one commit. Commit subjects follow Conventional
  Commits (`type(scope): Subject`), properly capitalized, subject line only, no
  body.
- Every commit SHALL pass `task all` on its own, in isolation, at the point it
  lands in history, never relying on a later commit to make an earlier one
  compile, lint clean, or pass tests.
- Every commit adds a complete, wired, tested increment.
- Every requirement and scenario in this change's specs is covered by a test, and
  every test added here traces to a scenario.
- OpenSpec artifacts for this change (this directory) are not committed until the
  change is archived.

## 1. Package rename

- [x] 1.1 Move `internal/runtimeartifacts` to `internal/resource` and
  `internal/runtimeartifacts/runtimeartifactstest` to
  `internal/resource/resourcetest`, updating every importer. Mechanical only: no
  identifier, signature, or behavior change beyond the package qualifier.
- [x] 1.2 Move `assets/resources` to `assets/resourcedata`, updating the
  generator, `cmd/exasol`, and the gitignored generated output path. Mechanical
  only.

## 2. Locator, Descriptor, and specification fields

- [x] 2.1 Add `Locator{Scheme, Location, Ref}` and `Descriptor{Locator, Sha256,
  Extract, Subpath}`, and a single `ParseURI` that parses the `@ref` and
  `#subpath` command-line grammar into a `Descriptor`. Replace the scattered
  `ParseGitURL`, `parsePresetURI`, and `IsExternalPresetURI` string handling with
  it. Test the suffix forms individually and combined, and that a specification
  declaring the same `ref` and `subpath` as fields resolves identically.
- [x] 2.2 _(landed with 2.1 in one commit: the `ref` field is unusable until
  its consumers read it.)_ Rename the specification's `resource_path` field to `subpath` and add
  the `ref` field, updating `assets/resourcedata/resources.yaml`. Define `subpath`
  as one literal path applying whenever resolved content is a directory, and
  delete the per-source-kind validation matrix it replaces. Test subpath
  selection within a clone, an extracted archive, and a local directory, and that
  a subpath escaping its resolved content is rejected.

## 3. Source contract

- [x] 3.1 Replace `Source.CanFetch`/`Fetch`-with-redirect and the optional
  `Identifier` interface with `Handles`/`Probe`/`Fetch`, where `Probe` returns
  `{Identity, Local}` and `Fetch` returns no path. Adapt all four sources. Probe
  errors propagate rather than being discarded. Test that a local directory sets
  `Local` and records no cache entry, that a local archive with `Local` set is
  still extracted into the cache, and that a probe failure is reported.
- [x] 3.2 Split identity from verification: identity is the declared checksum
  when set and the source's probe otherwise; verification and integrity become
  one content hash owned by the cache, applied on store and again on diagnosis.
  Test that a declared checksum identifies an artifact, that a source-stated
  identity is used when no checksum is declared, and that a mismatched download is
  rejected and not recorded as usable.

## 4. Per-source identity

- [x] 4.1 Resolve git identity from the commit before fetching, returning a full
  40-character `ref` without contacting the remote. Test cloning on first use,
  reuse on an unchanged commit, checkout by branch, tag, and SHA, update on an
  advanced ref, and that a pinned SHA with an existing entry contacts no remote.
- [x] 4.2 Add a container image probe returning the registry digest as identity,
  and remove `repackTarDeterministically`, `deterministic_tar.go`, and its test,
  since the stored archive's bytes no longer carry identity. Test that the
  registry digest identifies an image and that a second request with the same
  digest transfers nothing.
- [x] 4.3 Add an HTTP probe using a strong entity tag as identity where no
  checksum is declared, ignoring weak entity tags. Test that a strong validator
  lets a checksumless archive be reused, that a weak validator does not, and that
  an unidentifiable archive is re-fetched and logs the reason.
- [x] 4.4 Identify a local archive by path, size, and modification time, so a
  changed local archive is extracted again rather than serving a stale
  extraction. Test both the unchanged-reuse and changed-re-extract cases.

## 5. Cache storage

- [x] 5.1 Flatten cache storage to `artifacts/<hash(identity)>/`, excluding
  `Extract` and `Subpath` from the digest, and bump the cache schema version so
  prior entries are not reused. Test that two subpath selections of one source
  share a fetch, that extracted and unextracted views share a download, and that
  a resource with no identity does not collide with another. Test that JSON
  listing retains creation time, source URL, and identity.
- [x] 5.2 Stage every fetch and every extraction under `staging/<temp>/` and move
  the finished entry into place in one step, so an entry exists only when
  complete. Test that an interrupted materialization leaves no artifact reported
  as cached and that a later request materializes it again.
- [x] 5.3 Rename the `--partial-downloads` cleanup mode to `--incomplete` and make
  it remove interrupted materializations from the staging area. Test that cleanup
  reclaims an interrupted entry's space, that indexed artifacts survive it, and
  that preview mode reports without removing.

## 6. Generated resolved specification

- [x] 6.1 Emit a fully concrete resolved specification and blob directory per
  target platform under `assets/resourcedata/embedded/data/<goos>_<goarch>/`,
  with no embedding directive or expansion pattern remaining, blobs named with
  their real file extensions, and relative local locations rejected when not
  embedded. The generator writes no Go: remove its template, its `go/format`
  pass, and the per-resource identifier mangling. Test that the generated
  specification carries no build directives, that an embedded resource points at
  `embedded://` while an unembedded one points upstream, that a relative local
  location fails generation, and that generating for one platform leaves
  another's output unchanged. Prune the target platform's directory to exactly
  what the run references, since the wrapper embeds the whole directory and
  anything left behind would ship: test that data for a resource dropped from
  the specification disappears, that a resource skipped under placeholder-only
  mode disappears, and that another platform's data survives.
- [x] 6.2 _(landed with 6.1, 6.3, 7.1 and 7.2 in one commit: the resolved
  specification is what lets the runtime read embedded data, expand globs at
  build time, and list members, so none of them stands alone.)_
  Add `assets/resourcedata/embedded`: an untagged file declaring `Blobs`
  and `ResolvedSpec` with empty defaults, one hand-written build-tagged wrapper
  per supported platform embedding `all:data/<goos>_<goarch>`, and a tracked
  per-platform `.gitignore` so the package compiles before anything is
  generated. Add an `EmbeddedSource` resolving `embedded://` from that `fs.FS`,
  taking each blob's identity from the resolved specification, and delete the
  package-level registration maps and `Register`. Introduce
  `resource.Options`/`resource.New` as the constructor that accepts the
  specification and the blob FS, wire it at the composition root, and delete the
  blank imports in `cmd/exasol` and the two test packages. Confirm
  `internal/resource` still does not import the embedded package, so the
  generator cannot reach embedded data. Test that embedded data resolves without
  network access, that missing embedded data fails rather than falling back, and
  that two launcher versions embedding different content under one identifier
  resolve to distinct cache entries.
- [x] 6.3 Make placeholder-only generation emit upstream sources rather than
  placeholders, so the runtime no longer distinguishes embedding modes. Test that
  placeholder-only mode embeds nothing for an `embed: true` resource while still
  producing a resolvable specification, and that an `embed: always` resource is
  embedded regardless.
- [x] 6.4 Stop folding a declared `ref` back into the artifact URL now that
  sources take a `Locator`, deriving the cache key from the locator instead, so
  the revision stays a separate field end to end. Test that two refs of one
  repository remain distinct cache entries and that a ref declared as a field
  and one spelled in the URL still resolve alike.

## 7. Build-time glob expansion

- [x] 7.1 Make `glob` pattern-valued and expand it at build time into one
  `<group>/<member>` resource per match, each with its own blob and identity, and
  remove runtime glob matching. Test that a pattern expands into one resource per
  match, that an empty pattern is rejected, that files and directories are both
  valid members, and that repository metadata does not become a member.
- [x] 7.2 List group members by prefix scan over the specification and remove the
  build-time member registry. Test that listing materializes nothing, that an
  unknown group reports no members, that resolving one member leaves its siblings
  unmaterialized, and that an unknown member fails naming the member and group.

## 8. Resolver surface

- [x] 8.1 Rename `Manager` to `Resolver` and collapse its surface to
  `Resolve(ctx, id)`, `Resolve(ctx, Descriptor)`, and `List(prefix)`. Move
  copying to a free function, delete the constructors that `resource.New`
  supersedes, move the source and extractor registries from package variables
  onto the resolver, and migrate every consumer. Remove the nine
  `//nolint:paralleltest` markers naming the embedded-data and group registries,
  and confirm those tests pass in parallel. Markers caused by process-wide
  `PATH`, environment, working directory, or Cobra globals stay.
- [x] 8.2 Move platform selection onto the specification, flattened lazily per
  lookup. Test that a specification declaring artifacts only for other platforms
  still loads and fails only when that resource is requested, and that a
  platform-specific artifact still takes precedence over an `any` artifact.

## 9. Preset resource layering

- [x] 9.1 Add `Layer(spec)`, deriving a resolver that shares the cache and walks
  the specification chain, and read a preset's own resource specification while
  that preset is evaluated. Resolve both selected presets through launcher
  resources, then derive each evaluation layer directly from the launcher.
  Test that a preset-declared resource resolves during evaluation and not after,
  that two presets declaring one name each resolve their own, that neither can
  replace the other selected preset, and that an invalid preset specification is
  reported naming the preset.
- [x] 9.2 Allow a preset to override a launcher resource for the duration of its
  evaluation, and audit resolver references held in struct fields so a captured
  resolver cannot miss an override. Test that the override applies during
  evaluation, that the launcher's declaration applies again afterwards, and that
  the two declarations do not share a cached artifact.
- [x] 9.3 Resolve relative locations in a preset specification against the
  preset's own directory, and ignore embedding and expansion directives with a
  warning naming the directive and the preset. Test that a preset addresses its
  own content regardless of working directory, and that each ignored directive
  warns while the resource still resolves from its declared source.

## 10. Preset system cleanup

- [x] 10.1 Distribute shared deployment assets through the resource system,
  removing the `//go:embed all:shared/**` declaration and the hand-rolled
  recursive embedded-directory copier. Test that the same files are written into a
  deployment directory as before.
- [x] 10.2 Match a preset argument against known preset names before treating it
  as a location, demoting the syntactic path classifier to selecting the failure
  message, and collapse the preset reference union to one resolution path. Test
  that a plain name is not shadowed by a same-named local directory, that an
  unknown plain name lists available names, and that an unreachable location
  reports a fetch failure instead.

## 11. Documentation

- [x] 11.1 Update `doc/development.md` and `doc/architecture.md` for the resolved
  specification, the `<group>/<member>` naming convention, preset resource
  layering, and the renamed packages, and add `CHANGELOG.md` entries for the
  renamed cleanup flag and the cache rebuild.
