## Context

Built-in preset directories (`assets/infrastructure/*`, `assets/installation/*`) are embedded
via native `//go:embed all:infrastructure/**` in `internal/presets`. The runtime-artifact
resource cache (`internal/runtimeartifacts`) already fetches, caches, verifies, and (for
`embed: true` resources) embeds content from local directories, git repositories, and
archives, one catalog entry at a time. Presets are a whole category of directories, not a
single entry, so using the existing catalog as-is would mean one hand-written entry per
preset. This design adds a generic way for one catalog entry to stand for a whole matched
subset of its own resolved content, then applies it to presets specifically.

## Goals / Non-Goals

**Goals:**
- One resource-catalog entry can stand in for a whole category of subdirectories, regardless
  of whether that category currently lives in a local directory, a git repository, or an
  archive.
- Adding a new built-in preset directory needs no code or catalog edit.
- Built-in presets read through the same resource cache as any other artifact.

**Non-Goals:**
- Actually hosting presets outside this repository (this only removes the structural
  obstacle).
- Any change to how a preset is authored, selected, or behaves once resolved.
- Generalizing checksum/identity handling beyond what glob expansion and preset embedding
  themselves need.

## Decisions

### The glob pattern lives in `resource_path`, not the URL

**Decision:** `ResourceDefinition.Glob: true` marks an entry as a template; its artifact's
existing `resource_path` field holds the glob pattern to match once the artifact's URL is
resolved, instead of a single literal subpath.

**Rationale:** `resource_path` already exists to select a subpath within a resolved artifact,
and is already validated for path traversal. A glob pattern is a natural generalization of
"which subpath," not a new concept, so it reuses the same field rather than inventing URL
syntax (e.g. a `#pattern` fragment). This also keeps every artifact URL a plain, valid source
URL for its scheme, with no special-casing needed to strip a pattern back off before
dispatching to a source.

**Alternatives considered:** A `<url>#<pattern>` fragment, mirroring the `#subdir` convention
already used for external preset URIs (`add-preset-subdirectory-selector`). Rejected: that
convention exists specifically because a preset is supplied as a single CLI string with no
separate structured field available. A catalog entry in `resources.yaml` already has a
dedicated field for exactly this purpose, so cramming a pattern into the URL would duplicate
`resource_path`'s job instead of reusing it.

### A glob template is one resource; a member is a match within its own resolved result

**Decision:** `Glob: true` does not expand a catalog entry into separate resources. The entry
resolves through the exact same fetch/extract/embed pipeline as any other resource, as one
resource under its own resource ID. A member is resolved by matching the template's own
`resource_path` pattern against that resolved result's own entries (files or directories
alike) — never independently fetched, cached, or embedded.

**Rationale:** `Extract: true` on a glob template is unambiguous once nothing else claims a
competing meaning for it: it describes whether the template's own root needs unpacking to be
browsable, exactly as for any non-glob resource. The earlier alternative — expanding a template
into one concrete resource per match — left `Extract` on the template semantically overloaded
(does it describe the template's own root, or every matched child's independent re-archiving?)
and required a separate, synthetic resource ID per match. Treating `resource_path` as a
"template" applied lazily to the already-resolved root avoids both problems: there is inherent
ambiguity in "is the group archived, or the individual matched entries" once you allow
`Extract: true`, and resolving it in favor of the group keeps a glob template indistinguishable
from any other resource except for how its `resource_path` is interpreted.

**Alternatives considered:** Expanding a glob template into synthetic per-match resources named
`<template-id>/<match-name>`, each independently archived and embedded with its own
`extract: true`. Rejected: this doubled the concepts a glob template needed (a distinct
expansion pass at both build time and runtime, a `Group` field linking a match back to its
template, per-match cache entries) for no benefit over treating the pattern as applying to the
group's own resolved content once, on demand.

### One glob-matching operation for every source kind

**Decision:** A single, pure function matches a glob pattern against an already-resolved root's
own entries. It has no dependency on how the root was resolved — a local directory, a cloned
git repository, and an extracted archive all produce a plain directory to match against, so the
matching step itself never dispatches on source kind.

**Rationale:** The three source kinds already share one fetch/extract pipeline (`Manager.Get`);
once that pipeline has produced a resolved root, matching a pattern against its contents is a
pure filesystem operation with nothing left to know about where the root came from. Decoupling
"resolve the root" from "match within it" is also what lets a member resolve without any
independent fetch: `RequestMember` calls `Get` once for the whole group, then matches.

**Alternatives considered:** A per-source-kind dispatch (a git-specific clone-and-match
operation, a local-filesystem `filepath.Glob`, an archive-specific cache-backed operation).
Rejected: three near-identical implementations of the same "resolve, then match" shape is
exactly the duplication a single generic operation avoids.

### Git sources are recognized only by an explicit scheme, never a bare local path

**Decision:** A git source is identified only by `git@`, `git://`, or `https://`/`http://`
ending in `.git` — never by a bare local filesystem path, even one conventionally named with a
`.git` suffix.

**Rationale:** A bare local path is unambiguously a `file://`-equivalent source; treating a
`.git`-suffixed one as git instead would require every local-path caller to special-case it,
for a case (deliberately cloning a local bare repository by path alone) with no real use case
in this system.  Anyone who wants git semantics for a local repository can address it with an
explicit scheme.

### Preset directory reads move behind a new leaf package

**Decision:** A new `internal/launcherpaths` package holds path-resolution logic with no
dependency on `internal/runtimeartifacts`, `internal/presets`, or `internal/config`, extracted
ahead of switching preset reads onto the resource cache.

**Rationale:** `internal/presets` reading preset directories through `internal/runtimeartifacts`
would otherwise create an import cycle wherever both packages currently depend on shared path
logic. Extracting the dependency-free piece first breaks the cycle without restructuring either
package's own responsibilities.

### One resource manager per process, carried on context

**Decision:** `cmd/exasol` builds exactly one `*runtimeartifacts.Manager` at startup and attaches
it to the root `context.Context` (`runtimeartifacts.NewContext`/`FromContext`), instead of each
call site (preset reads, the deployment backend, external preset resolution) building its own.
`Manager.GroupMembers` gives a caller a way to list a glob template's build-time-registered
member names directly, so `internal/presets` no longer re-derives that by re-parsing a spec of
its own.

**Rationale:** Every one of these call sites already receives `context.Context` as a parameter,
so the shared Manager rides along on plumbing that already exists everywhere it's needed, rather
than adding a second parameter thread alongside it. This also means `internal/presets` no longer
needs to know a cache or spec exists at all — it asks the Manager to resolve a resource or list a
group, exactly like any other caller.

**Alternatives considered:** A lazily-initialized package-level singleton in
`internal/runtimeartifacts` (no explicit construction site). Rejected: it hides where and when
the Manager is built, and the process already has one obvious place — `cmd/exasol`'s `Execute()`
— to build it explicitly once. Threading a `*runtimeartifacts.Manager` as an explicit parameter
through every intervening function (rather than via context) was also considered and rejected:
most of those functions have nothing to do with resource resolution and would only be carrying
the parameter further down, which is exactly what `context.Context` is already there to avoid
repeating.

## Risks / Trade-offs

- **A glob template's root is fetched once whether resolving the whole group or a single
  member.** Mitigation: `Manager.Get` already caches by content identity, so a member lookup
  right after a group lookup (or vice versa) reuses the same cached root rather than
  re-fetching.
- **The build-time generator's embedded archive must be filtered to only the matched entries,
  not the root's full content, or a large unmatched sibling directory would bloat every
  binary.** Mitigation: `resourceembedder` matches the pattern once at generation time and
  archives only the matched entries, discarding the rest before embedding.
