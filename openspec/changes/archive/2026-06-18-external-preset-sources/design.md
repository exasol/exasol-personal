## Context

Presets are configuration bundles (infrastructure or installation) that the launcher materialises into a deployment directory before executing Terraform. `PresetRef` currently identifies a preset either by an embedded binary asset name or a bare filesystem path. The CLI classifies an argument by heuristic path detection (leading `.`, `~`, or a path separator).

The project already contains `internal/runtimeartifacts`, a caching resource manager that downloads and extracts tool binaries (e.g. OpenTofu) from HTTP/HTTPS URLs. It supports checksum verification, atomic caching, and platform-specific artifact selection. Extending this existing infrastructure — rather than building a parallel fetch stack — keeps the codebase coherent and provides caching immediately.

The Manager is currently re-created on every call inside `ResolveBinaryPath` in `internal/tofu/resource.go`. This is an existing design gap: re-instantiation means re-parsing the spec and re-initialising the cache on every binary path request.

## Goals / Non-Goals

**Goals:**
- Accept `file://`, `https://`, `http://`, and git repository URIs (`https://*.git`, `http://*.git`, `git://`, `git@`) as preset sources.
- Plain names continue to resolve to embedded presets; bare filesystem paths continue to resolve locally.
- External presets are fetched and cached via an extension of `runtimeartifacts`, with no new parallel download infrastructure.
- Git operations use a pure-Go library (`go-git`) so no external `git` binary is required at runtime.
- `PresetRef` resolution produces a local path; all downstream code (extraction, manifest reading, validation) is unchanged.
- The `runtimeartifacts.Manager` is an implementation detail of the library; callers in `cmd/` and tests that don't involve resource resolution never construct one directly.

**Non-Goals:**
- Recursive preset dependencies or preset composition.
- User-specified checksums for remote archive preset sources (follow-up work; noted below).
- Custom git authentication (tokens, passwords, SSH key files, per-host credential helpers); `git@` SSH URLs authenticate via the SSH agent only (`SSH_AUTH_SOCK`).

## Decisions

### 1. External preset resolution produces a local path, not a new PresetRef kind

**Decision:** When a user provides an external source URI, it is resolved to a local path before `PresetRef` is constructed. The result is a `PresetRef{Path: "<resolved-path>"}`. All downstream code — `ExtractPreset`, `readInfrastructureManifestFromPreset`, `readInstallManifestFromPreset`, `ValidatePresetSelection` — is untouched.

**Rationale:** Limiting the change surface to the classification and resolution step avoids touching the materialisation pipeline. The resolved path is a stable local directory, so all code that currently works with `PresetRef.Path` works without modification.

**Alternatives considered:** Extending `PresetRef` with a `SourceKind` discriminant and teaching `ExtractPreset` to dispatch on it. Rejected because it requires touching every site that reads manifests or extracts content — a much larger change with no functional advantage over resolving to a path first.

### 2. Classification at the CLI boundary; resolution in the library

**Decision:** URI classification and resolution are handled by a single function, `resolvePresetRef`, in `cmd/exasol/util.go`. Plain names (no path separators or URI scheme) return immediately as `PresetRef{Name: arg}` without any network or filesystem access. Everything else — bare filesystem paths and external URI scheme arguments — causes `resolvePresetRef` to construct a `Manager` and pass it to `ResolvePreset`, which calls `manager.Get` and returns the resolved local path; `resolvePresetRef` wraps this as `PresetRef{Path: "<resolved-path>"}`. All command `RunE` handlers use `resolvePresetRef`.

`@<ref>` parsing in `ResolvePreset`: for `git@` SCP-style URLs (which already contain one `@` before the hostname), the ref separator is the last `@` that appears after the `:` path separator (e.g. `git@github.com:org/repo.git@main` → URL: `git@github.com:org/repo.git`, Ref: `main`). For scheme-based URLs (`https://`, `http://`, `git://`), the last `@` in the URL is tentatively treated as the ref separator: the candidate ref is stripped first, and `IsGitSourceURL` is called on the remaining URL. If the stripped URL is not a git source but a ref was found, the argument is rejected with an error.

The preset type (infrastructure/installation), known from the argument's position on the command line, is passed to `ResolvePreset` so that the post-fetch manifest check verifies the correct file.

**Rationale:** Keeping classification in `cmd/` and resolution in the library means the library doesn't depend on CLI concerns, while the CLI doesn't need to understand caching or fetching. The two-function split avoids triggering fetches in completion and display paths that only need to know whether an argument is a name, a path, or a URI.

### 3. runtimeartifacts.Manager is used for preset fetching; constructed at the call site

**Decision:** Preset fetching is implemented by constructing a `runtimeartifacts.Manager` in `resolvePresetRef` (`cmd/exasol/util.go`) and passing it to `deploy.ResolvePreset`. This is the same `Manager` used for tool binaries (e.g. OpenTofu), so preset sources share the same cache directory, locking, and eviction logic with no duplication.

`ResolvePreset` (`internal/deploy/preset_external.go`) accepts a `*runtimeartifacts.Manager` parameter, constructs a `ResourceDefinition` from the URI, and calls `manager.Get(ctx, def, presetType)`. The Manager handles all source dispatch, caching, and extraction; `ResolvePreset` only handles URI parsing and post-fetch manifest verification.

The tofu binary path is resolved when the tofu backend (`tofu.Config`) is instantiated — not at program startup. `ResolveBinaryPath` is updated to use a shared Manager rather than creating a new one on every call, and the resolved path is stored on `tofu.Config` so that subsequent calls to `TofuBinaryPath` return the cached path without re-requesting.

Tests that exercise code paths not involving resource resolution (e.g. config parsing, manifest validation) are unaffected and do not need to construct or mock a Manager.

**Rationale:** Reusing `runtimeartifacts` for preset fetching avoids a parallel download stack and gives preset sources caching, checksum verification, and platform key resolution for free. Passing the Manager as a parameter to `ResolvePreset` keeps the function independently testable without global state. Lazy tofu resolution means commands that don't use tofu (e.g. `exasol presets list`) incur no resolution overhead.

### 4. runtimeartifacts is refactored around Source/Extractor interfaces, then extended

**Decision:** `internal/runtimeartifacts` is refactored and extended:

- **Source/Extractor abstraction**: `download.go` is replaced by two interfaces — `Source` (URL detection and resource fetch, returning a redirect path when the resource already resides locally rather than writing to the destination) and `Extractor` (archive detection and extraction) — and a set of concrete implementations in separate files: `HttpSource`, `FileSource`, `GitSource`, `TarGzExtractor`, `ZipExtractor`. `Manager` iterates `sources` and `extractors` slices; new source or extractor types are added by appending to these slices.

- **Optional content identity**: Sources that can determine a stable content identity before fetching implement an optional `Identifier` interface. The Manager calls it when a definition carries no explicit checksum, using the returned value as a synthetic checksum to drive cache hit/miss decisions. `GitSource` uses this to resolve the target commit hash via remote ref listing; `FileSource` uses it to produce a stable cache key for local directories.

- **Manager.Get for ad-hoc definitions**: `Manager.Request` is refactored to extract its core resolution logic into a new `Manager.Get`, which accepts a definition directly and resolves it without requiring pre-registration. `Manager.Request` becomes a thin wrapper that looks up the definition from the static spec by ID and delegates to `Manager.Get`. The preset resolver calls `Manager.Get` directly with its runtime-constructed definitions. No spec merging is required.

- **`file://` URIs handled by FileSource**: `FileSource` accepts `file://` URIs and bare local paths. `Source.Fetch` returns a `(redirectPath string, err error)` pair; a non-empty `redirectPath` means the caller should use that path directly rather than `dstPath`. For local directories and archive files `FileSource.Fetch` resolves the path to an absolute value and returns it as `redirectPath` without writing anything to `dstPath`. The Manager's extractor pass reads from `redirectPath` directly when extracting archive files. The Manager records `redirectPath` in the cache index entry (`RedirectPath` field); on a cache hit it stats `redirectPath` directly to verify the source directory still exists. `FileSource` also implements `Identifier`: for directories it returns a SHA-256 of the absolute path so the cache key is stable; for archives it returns an empty string to preserve the no-checksum re-fetch policy.

- **No-checksum archives are always re-fetched**: An archive `ArtifactSpec` with an empty `Sha256` has no stable content identity and cannot be cached reliably. Every `Get` call for such a spec triggers a fresh download; any existing cache entry for that URL is replaced. A log message is emitted at `Info` level each time.

- **ZIP format support**: `ZipExtractor` recognises and extracts `.zip` archives alongside the existing `TarGzExtractor`.

- **Platform-independent artifact key**: a new sentinel key `"any"` is supported in the `Artifact` map. When the Manager cannot find an entry for the current platform, it falls back to `"any"`. Preset `ResourceDefinition`s use `"any"` since preset content is identical across platforms.

- **Conditionally optional checksum**: `ArtifactSpec.Sha256` is optional for programmatically-constructed definitions. The `ParseSpec` validator continues to require a non-empty `Sha256` for every archive-type entry in YAML-parsed specs (`resources.yaml`). Runtime-constructed specs bypass the validator.

**Rationale:** The Source/Extractor abstraction makes each fetch and extraction strategy independently testable and eliminates the large `download.go` switch statement. `Manager.Get` closes the API gap for runtime definitions without requiring definitions to be pre-registered or merged into the static spec. Returning the resolved path directly for `file://` sources means source directory changes are reflected immediately on the next run. The no-checksum re-fetch policy prevents silent use of stale content; the log message makes this behavior visible.

**Future work:** Allow users to annotate remote archive URLs with a checksum so that remote archives can be reliably cached without requiring a git repository.

### 5. Git support added via GitSource; refs embedded in URL as @-suffix

**Decision:** Rather than a parallel `GitArtifactSpec` type, git source detection is based on URL format: URLs starting with `git@` or `git://`, or ending with `.git` (including `http://*.git`), are treated as git sources and routed to `GitSource`. Local directories, including local git repositories, are handled by `FileSource` via a `file://` URI — `GitSource` handles only remote git URLs.

Git refs (branch, tag, or commit SHA) are communicated to `GitSource` via an `@<ref>` suffix appended to the URL (e.g. `https://github.com/org/repo.git@main`). `GitSource.Fetch` parses the suffix and resolves it against the remote's ref list before cloning.

For git sources:
- The ref is optional; omitting it clones the remote's default branch HEAD.
- When ref is a branch or tag name, `remote.ListContext` resolves it to the canonical ref name for a shallow clone (`Depth: 1`).
- When ref is a full 40-character lowercase hex commit SHA, `getRefName` scans the remote refs by hash value. If a named ref points to that commit, a shallow clone is used. If no named ref points to it, a full-depth clone is performed and the working tree is hard-reset to the target SHA.
- `Sha256` must be empty (the resolved commit hash is the content identity; git sources derive cache entries from the commit, not a file checksum).
- `DownloadPath` is unused.
- `ResourcePath` is honoured: when set, it selects a subdirectory within the cloned repository as the resolved path.

`ArtifactSpec.Sha256` validation in `ParseSpec` enforces: git sources must have empty `Sha256`; non-git sources must have non-empty `Sha256`.

**Rationale:** URL-embedded refs are the natural idiom already used by tooling like `go get` and `pip install`. Keeping git-source detection on the URL means callers do not need to set a separate source-kind field.

**Alternatives considered:** A Go interface (`ArtifactSource`) with two implementations. Rejected because Go interface dispatch for a two-case discriminant is more complex to parse from YAML and adds boilerplate without benefit.

### 6. Preset ResourceDefinitions are constructed in internal/deploy

**Decision:** Preset `ResourceDefinition`s are constructed at runtime from the URI and preset type provided by the CLI. They live in `internal/deploy/preset_external.go`, not in a separate `presetresolver` package. The resolver calls `manager.Get(ctx, def, presetType)` directly.

The preset type parameter (infrastructure/installation) determines which manifest filename to verify after the path is resolved. The resolver returns the resolved local path to the CLI, which wraps it in `PresetRef{Path}`.

`needsExtraction` in `preset_external.go` sets `ResourceDefinition.Extract` based solely on the URI's extension: it returns `true` only when the URI ends with a recognised archive extension (`.tar.gz`, `.tgz`, `.zip`), regardless of the URI scheme.

**Rationale:** Keeping the resolver in `internal/deploy` avoids a new package while staying outside `cmd/`. There is no import cycle because `internal/deploy` already imports `internal/runtimeartifacts`.

## Risks / Trade-offs

- **go-git adds a dependency** → `go-git` is a well-maintained, widely used library. The ref-resolution + shallow-clone surface area is small and well-covered by its API.
- **Unversioned git refs (branch names) always require a remote call to resolve** → The call is fast (list-refs only, no content transfer). Users who want reproducible builds should supply a full commit SHA.
- **No-checksum archive sources are always re-fetched** → This is an intentional, visible behavior (logged at Info). Users who experience the latency should switch to a git source or wait for future checksum annotation support.
- **`"any"` platform key fallback** → The fallback is only triggered when no platform-specific entry exists; existing tool binary entries in `resources.yaml` are unaffected.
- **`file://` directory presets return the original path directly** → The Manager records the source directory path in the cache index and returns it unchanged. Modifications to the source directory are reflected immediately on the next run, on all platforms.
- **`file://` archive presets are always re-extracted** → Because they carry no checksum, the no-checksum re-fetch policy applies. The re-extraction cost is expected; users who want caching should use a checksum-annotated remote archive (future work).

## Migration Plan

1. Refactor `runtimeartifacts`: replace `download.go` with `Source`/`Extractor` interfaces; extract `HttpSource` and `TarGzExtractor`; refactor `Manager.Request` into `Manager.Get(ctx, def, resourceID)` + `Manager.Request(id)`.
2. Add `FileSource` and `file://` support; wire into `sources` slice.
3. Add `ZipExtractor`; wire into `extractors` slice.
4. Add library-level preset resolver in `internal/deploy/preset_external.go`; update `cmd/exasol/util.go` to detect URI schemes and call `ResolvePreset`; wire preset type through CLI.
5. Add `go-git` dependency; implement `GitSource` with ref resolution and shallow clone; add `Ref` field and git validation to `ArtifactSpec`; wire `GitSource` as first entry in `sources`; extend preset resolver to handle git/HTTP URIs.
6. Add docs and improve unknown-preset error message.

No changes to `ExtractPreset`, `ValidatePresetSelection`, manifest readers, or any other downstream code. No config file format changes. No flag renames. Rollback: revert steps 4–6; `runtimeartifacts` changes are purely additive.
