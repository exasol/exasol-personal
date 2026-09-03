# Design

## Guiding rule

Resolve every decision at the earliest point it can be resolved, and let each
later stage read a concrete answer rather than re-derive it. Most of the current
complexity is conditionals that exist because a declaration says one thing to the
build and a different thing to the runtime.

## Two specifications, one schema

The authoring specification and the resolved specification share a schema. The
resolved one is the authoring one minus the two build directives:

```
authoring = resolved + { embed, glob }
```

The build eliminates `embed` and `glob`. It does not transform the schema, so a
hand-written specification and a generated one are read by the same parser, and a
preset author writes exactly what a non-embedded launcher resource looks like.

```
  resources.yaml (authoring)
        |
        |  resourceembedder, run from the repository root, per GOOS/GOARCH
        |    expands glob patterns into per-member resources
        |    fetches, hashes, and stages blobs under their real extensions
        |    eliminates embed and glob
        v
  data/<goos>_<goarch>/resolved.yaml  +  data/<goos>_<goarch>/blobs/...
        |
        |  hand-written, build-tagged wrapper reads them
        v
     runtime
```

The generator emits **data only**. It writes no Go, which removes its template,
its `go/format` pass, and the per-resource identifier mangling that came with
them.

Because generation always runs, `SKIP_EMBED` stops existing at runtime. It
becomes a generator decision:

| authoring        | SKIP_EMBED | resolved URL     |
| ---------------- | ---------- | ---------------- |
| `embed: false`   | either     | upstream         |
| `embed: true`    | set        | upstream         |
| `embed: true`    | unset      | `embedded://...` |
| `embed: always`  | either     | `embedded://...` |

The runtime cannot tell which happened, and does not need to.

## How embedded data reaches the runtime

Embedded data is addressed as an ordinary `embedded://` source, so the question
is only how the bytes and their identities get from the build into a running
binary. Two shapes were considered.

The first keeps today's mechanism: the generated file declares `//go:embed`
variables and an `init()` calling `Register(id, data, hash)` into a registry.
The second carries each blob's hash in the resolved specification as its
`sha256`, and hands the resolver an `fs.FS` over the blob directory.

The second is chosen. The deciding argument is that the first cannot remove the
package-level state: an `init()` in another package has to call *something*
package-scoped, so `Register` stays global however the storage is arranged, and
with it the tests that cannot run in parallel. Two further reasons: the blob
hash travelling in the descriptor is the same "resolve it once, let later stages
read a concrete answer" move the rest of this change is built on, and it retires
the blank-import-for-side-effect that `cmd/exasol` and two test packages
currently need, whose failure mode is a resolution-time "no embedded data
registered" that names nothing about the missing import.

### The wrapper is hand-written, not generated

`//go:embed` takes a compile-time literal path, so the platform cannot be
factored out of it, and platforms coexist in the tree because generating for one
must leave the others alone. That leaves one small build-tagged wrapper per
supported platform. Writing those by hand rather than generating them is what
lets the generator stop emitting Go.

```
assets/resourcedata/embedded/
  embedded.go                   declares Blobs and ResolvedSpec, empty defaults
  embedded_linux_amd64.go       //go:build linux && amd64
  embedded_darwin_arm64.go      ... one per supported platform
  data/
    linux_amd64/
      .gitignore                tracked; the rest of the directory is not
      resolved.yaml             generated
      blobs/...                 generated
```

Each wrapper embeds its own platform directory and reads the specification out
of it:

```go
//go:embed all:data/linux_amd64
var platform embed.FS
```

`all:` matters twice over: it is what makes the tracked `.gitignore` count as a
match, and a bare `//go:embed` of a directory with no matching files is a
compile error. That one tracked file per platform directory therefore does two
jobs, keeping the directory alive through a clone and keeping the package
compiling before anything has been generated. It is the same per-directory
`.gitignore` idiom already used under `assets/resourcedata/generated` and
`assets/tofubin`.

Embedding the directory rather than each file by name means generation has to
prune it: anything a previous run left behind is now shipped rather than merely
ignored. Under the per-file directives this did not matter, which is why debris
was tolerated. It matters most under placeholder-only generation, whose whole
purpose is to avoid embedding, and which a stale image blob would defeat
entirely. Pruning costs a local copy from the resource cache on the next full
generation, not a re-download.

A platform with no wrapper falls back to the empty defaults in `embedded.go`, so
targets the project does not ship still build. Neither shape turns a missing
generation step into a compile error, since both keep an untagged declaration of
the symbols; a missing generation remains a runtime failure, as it is today.

### The blob FS is injected, not imported

`internal/resource` never imports the embedded package. `cmd/exasol` and the
test helpers do, and pass the FS in:

```go
resolver, err := resource.New(resource.Options{
    Spec:  embedded.ResolvedSpec,
    Blobs: embedded.Blobs,
})
```

This is what keeps the generator's independence structural rather than
conventional. `tools/resourceembedder` imports `internal/resource`; if that
package in turn imported the embedded data, the generator would have the
previous build's bytes within reach, and the requirement that it always fetches
independently would rest on discipline alone. Injecting instead means the
generator constructs a resolver with no `Blobs` at all, and its embedded source
physically cannot satisfy a fetch.

## Locator, Descriptor, and field ownership

```go
// What a Source sees. Nothing here is about presentation.
type Locator struct {
    URL string // the location, with any "@ref" and "#subpath" suffix removed
    Ref string // tag, branch, commit, image tag, version id
}

// Scheme is derived rather than stored. Sources hand the URL to their own
// libraries intact (go-git wants git@host:path, the image library wants
// docker://...), so a stored scheme would duplicate a prefix of URL rather
// than replace it, and an SCP-style git URL carries no scheme at all.
func (l Locator) Scheme() string

// What the Resolver sees.
type Descriptor struct {
    Locator Locator
    Sha256  string
    Extract bool
    Subpath string
}
```

Every field has exactly one consumer, which is the test that the decomposition is
real rather than rearranged:

```
  Descriptor { Locator, Sha256, Extract, Subpath }
                  |       |        |        |
                  |       |        |        +--> Resolver, after materialization
                  |       |        +-----------> Resolver, selects an Extractor
                  |       +--------------------> Cache, verification on store
                  +----------------------------> Source
```

A specification declares `url`, `ref`, and `subpath` as named fields. The
`url@ref#subpath` string grammar remains a command-line shorthand, since a user
types a location as one argument, and `ParseURI` is the single function that
knows it. Both forms produce a `Descriptor`.

`subpath` has one meaning everywhere: a literal path selected from resolved
content, applying whenever that content is a directory. That rule replaces the
current validation matrix over source kinds, which existed to police a
distinction that no longer holds now that git checkouts, extracted archives, and
local directories are all directories.

## Identity, verification, and integrity

Three questions that the current `Sha256` field answers at once:

| concern      | owner  | question                              | used for  |
| ------------ | ------ | ------------------------------------- | --------- |
| identity     | source | is upstream still the same thing?     | cache key |
| verification | cache  | are these the bytes that were declared? | on store  |
| integrity    | cache  | did my copy rot since I wrote it?     | diagnostics |

Verification and integrity are the same content hash at two times, so they are
one implementation in the cache, which already owns bytes on disk. No source
implements hashing. What remains source-side is intrinsic verification the
protocol performs anyway: git verifies its object graph while cloning, and the
image copy verifies the manifest digest while transferring.

Identity is derived, never authored:

```
identity = "sha256:" + descriptor.Sha256   if set
         = source.Probe(locator)           otherwise
```

The defect being removed is not that a content hash serves as identity, which is
honest. It is that non-hash identities were disguised as one: a path hash for
local files, a git commit assigned to a field named `Sha256`. After the split the
field only ever holds a real sha256, so its name becomes accurate.

Identity comes from upstream when upstream names the content, and from a
build-time hash when the generator produced it:

| source           | identity                                  |
| ---------------- | ----------------------------------------- |
| embedded blob    | `sha256` of the blob, generator-written   |
| pinned http      | declared `sha256`                         |
| unpinned http    | strong entity tag, or none                |
| git              | resolved commit                           |
| container image  | registry manifest digest                  |
| local directory  | none; `Probe.Local` is set instead        |
| local archive    | path, size, and modification time         |

Two consequences follow.

`repackTarDeterministically` and `deterministic_tar.go` are removed. Their own
comment states the purpose: replacing host-dependent metadata so the archive is
identical on every platform. That normalization exists solely to make a
hash-of-file usable as identity when Windows alters permission bits. Once the
registry digest supplies identity, the tar's bytes stop mattering for caching.
Nothing else depends on them: the image loader accepts either archive, and cache
integrity compares against the hash recorded at store time rather than a
cross-platform constant.

The generator's `tarGzDirectory` keeps zeroing timestamps. Determinism is
required exactly where the hash is the identity, which is true of a generated
glob blob and untrue of a transported image.

## Probe replaces both redirect and Identifier

```go
type Probe struct {
    // Identity changes exactly when the content changes. Opaque to the
    // resolver; each source formats its own. Empty means the source cannot
    // tell, and the resource is refreshed on every request.
    Identity string

    // Local names a path that already holds the artifact. When set, Fetch is
    // never called and the cache stores no copy of the artifact itself.
    Local string
}

type Source interface {
    Handles(loc Locator) bool
    Probe(ctx context.Context, loc Locator) (Probe, error)
    Fetch(ctx context.Context, loc Locator, dst string) error
}
```

`Probe` is what a source knows before transferring, and identity and locality are
both that. Making it one method rather than two keeps `Fetch` honest: it places
content at `dst` and returns no path, so no source can tell its caller to ignore
the destination it was given.

`Probe.Local` carries both facts in one field, which resolves a case the redirect
conflated. A local archive is external as a download but its unpacking is cached:

```
  Local set, no extract  ->  return Local plus subpath. No index entry.
  Local set, extract     ->  extract from Local into the cache. Index the unpack.
  Local empty            ->  fetch, verify, maybe extract, index.
```

Errors from `Probe` propagate. The current code discards them, silently
downgrading a failed `ls-remote` into "no checksum", which then means "re-fetch
every time".

Entity tags are used only where no checksum is declared, so they can only improve
on the current always-re-fetch behavior. Weak entity tags are ignored, because
they promise semantic equivalence rather than byte equality. The failure mode of a
strong entity tag is a spurious re-fetch, never stale content.

A full 40-character `ref` is returned by `Probe` without a network call. A
floating ref costs one `ls-remote` per resolution, including cache hits, which is
the only way to know whether the cache is stale, and is what the current code
already does.

## Cache layout and staging

```
  <root>/artifacts/<identity-digest>/artifact.tar.gz
  <root>/artifacts/<identity-digest>/unpack/
  <root>/staging/<temp>/
```

The current layout nests resource ID, GOOS, and GOARCH above the digest, all of
which the digest already covers. Flattening removes the nesting, leaves no empty
parent directories behind after cleanup, and makes an entry one directory to
remove.

The digest is `hash(identity)` alone. `Extract` and `Subpath` are presentation,
not identity, so one clone serves every subpath selection of it and one download
serves both an extracted and an unextracted view. When identity is empty, the key
falls back to the locator, so resources with no identity do not collide.

The index record holds only what is stored: identity, the resources sharing the
entry, the declared checksum, the artifact's filename, and retention metadata.
`extract` and `subpath` never reach it, and neither do any derived paths, which
the resolver recomputes from the descriptor on every request. That is what lets
two descriptors differing only in presentation share one record instead of
overwriting each other's.

Two consequences. Extraction cannot ride a whole-entry atomic rename, since
`unpack/` may be added to an entry that already holds the artifact; staging the
unpack and renaming that directory alone keeps the guarantee per item. And a
cache hit is confirmed against the filesystem, not the index alone, so a user
who deletes part of the cache gets a refetch rather than a dangling path.

Every source stages into `staging/<temp>/`, including the cheap ones, since a
same-filesystem rename costs nothing and uniformity removes a special case.
Extraction stages too. Renaming the entry directory as a whole, rather than the
artifact within it, makes the existence of `artifacts/<digest>/` mean the entry is
complete, which is a stronger invariant than an index assertion. Today
`HttpSource` stages inside the entry directory and nothing writes to `downloads/`
at all, so `cache clean --partial-downloads` cleans a directory that is never
populated.

The cache schema version is bumped and prior entries are not reused. No migration
is performed, but a version mismatch must never leave the launcher unusable, so
the two directions are handled differently.

An index from an *earlier* launcher describes a layout this one no longer reads.
Its entries are dropped and a fresh index replaces them, and the launcher carries
on. The stored content stays on disk, referenced by nothing, so the launcher
tells the user it can be reclaimed with `cache clean --all`, using the same
call-to-action mechanism as any other next-step guidance: standard error, and
suppressed under `--json`.

An index from a *later* launcher cannot be interpreted safely, so it is refused.
The refusal names both actions that resolve it: upgrade, or discard the cache
with `cache clean --all`.

Rejecting an earlier index instead, as a strict version equality check would,
would make every upgrade fail on its first resource resolution with nothing but
a version number to go on. That is the failure this handling exists to prevent.

## Glob becomes build-time expansion

The term is kept, the mechanism changes. `glob` becomes pattern-valued, which
removes the `resource_path` overload at its source rather than working around it:

```yaml
infrastructure-presets:
  embed: always
  glob: "*"
  artifact:
    any: { url: assets/infrastructure }
```

expands to one resource per match, each with its own blob and its own cache entry:

```yaml
infrastructure-presets/aws:
  extract: true
  artifact:
    any:
      url: "embedded://infrastructure-presets/aws"
      sha256: b3014316112ae5dc...
```

|                        | current                       | this change              |
| ---------------------- | ----------------------------- | ------------------------ |
| when it runs           | every request                 | once, at build time      |
| runtime representation | one group; members are subpaths | independent resources  |
| cache entries          | one per group                 | one per member           |
| fetching one member    | materializes the whole group  | materializes that member |
| listing members        | separate build-time registry  | prefix scan of the spec  |

The two halves of the current concept disagree today: members resolve by
re-matching at runtime while listing reads a registry, and listing returns nothing
for a group that was never embedded. Expansion makes both halves read the
specification.

A group is `group/member` by convention, and `List` is a prefix scan. This repo
prefers convention over configuration; the convention is stated in the
capability.

The cost is that a glob cannot appear in a preset layer, since it is build-time
only. Globbing a non-embedded source has no caller today, and the listing half of
the concept was already build-time only.

## Preset resource layering

Resolution is a function that can run at two times, with different capabilities:

```
  build time                          runtime, preset layer
  can embed, fetch now                cannot embed, fetch lazily
       |                                       |
       v                                       v
  generated resolved spec  <--base--  Resolver  --layer-->  preset resources.yaml
```

`Layer` derives a resolver sharing the cache. Lookups walk the chain, base last.
Overrides are permitted, including of launcher resources, and the layer is
discarded when the preset's evaluation ends. Cache identity already keeps two
presets that define the same name from colliding, since identity and locator
differ.

Infrastructure and installation preset locations are resolved through the base
resolver before either layer applies. Their evaluation resolvers are siblings,
each derived directly from the base, so one preset cannot replace the other or
leak resources into the other preset's evaluation.

The scoped resolver must reach everything running during that evaluation. The
existing context-carried resolver is the mechanism; resolver references held in
struct fields are the hazard to find, since one captured before the preset was
entered would silently miss the override.

`embed` and `glob` in a layer are ignored with a warning naming the key and the
preset. The intent is clear and the directive cannot apply after the binary is
built.

Relative locations resolve against the directory of the specification that
declared them: the repository root for the built-in specification, which the
generator makes true by running there, and the preset's own directory for a
preset. Command-line arguments are made absolute when parsed, since an argument
is not specification content. The generator rejects a relative local location
that would survive into the resolved specification, because a built binary has an
arbitrary working directory and no repository root.

## Platform

Platform is a property of a specification, not of the resolver. It is applied
lazily per lookup, so a preset declaring artifacts for three platforms still
loads on a fourth and fails only if that resource is actually requested:

```go
spec := resource.LoadSpec(raw, platform) // parses, flattens nothing
d, err := spec.Lookup("provider-helper") // flattens this one only
```

The platform is always the host's, except during cross-compilation, where the
generator supplies the target. The generated specification is already
single-artifact, so its lookup ignores the platform.

## Resolver

The resolver is the orchestrator: the only component that knows the order of the
pipeline. It holds the specification chain, the source registry, the extractor
registry, and the cache, and it owns no transport, unpacking, or storage.

```go
Resolve(ctx, id string)    (string, error)
Resolve(ctx, d Descriptor) (string, error)
List(prefix string)        []string
Layer(spec) *Resolver
```

Copying a resolved path is a free function rather than three more methods, since
copying was never resolution.

It stays a concrete struct with no interface. Every test double in the repository
today is a fake artifact on disk rather than a fake resolver, because the seam
that matters is `Source`, and the dependencies that are genuinely unavailable in a
test sit behind it. Moving the source, extractor, and embedded-data registries
from package-level variables onto the resolver gives per-test isolation. That
removes the `//nolint:paralleltest` markers that exist only because of those
registries. Markers caused by process-wide `PATH`, environment, working
directory, or Cobra globals are untouched by this change.

## Preset argument resolution

Matching names before locations preserves today's behavior in every case except
the message for an argument that is neither:

| argument    | today                          | this change                    |
| ----------- | ------------------------------ | ------------------------------ |
| `aws`       | not a URI or path, so a name   | a known name                   |
| `./aws`     | path-like, so a path           | not a name, so a location      |
| `git://x`   | a URI, so fetched              | not a name, so a location      |
| `awsx`      | a name, fails later            | neither; message is chosen     |

The syntactic classifier survives, demoted from a correctness gate to a message
selector. A directory named `aws` in the working directory cannot shadow the
`aws` preset, because a plain name never reaches location resolution. Getting the
classifier wrong now degrades a message rather than selecting the wrong preset.

## What is preserved

The parts that are risky to rewrite are kept and adapted rather than replaced:
the go-git transport, the image copy, the tar and zip extractors, the directory
lock, the atomic index write, and retention, cleanup, and diagnostics. These
survive largely intact.

The resolution pipeline, the validation matrix, and the registries are
rewritten. The resolver tests are re-derived from the scenarios in this change
rather than ported from semantics that are being removed.

## Naming

`internal/resource` is the code, so it is named for the call site.
`assets/resourcedata` holds the authoring specification, the generated
specification, and the blobs, which keeps generated output and a large image
blob out of `internal/` and inside a directory that is already ignored by
version control. The shared prefix ties them together and neither name has to
defend itself against the other.
