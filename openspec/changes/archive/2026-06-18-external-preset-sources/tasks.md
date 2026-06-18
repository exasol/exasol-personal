## 1. Refactor runtimeartifacts — Source/Extractor abstraction

- [x] 1.1 Define `Source` interface (URL detection and resource fetch with optional redirect-path return) and `Extractor` interface (archive detection and extraction) in `manager.go`
- [x] 1.2 Extract HTTP download logic from `download.go` into `HttpSource` in `http_source.go`; add `http_source_test.go`
- [x] 1.3 Extract tar.gz extraction logic from `download.go` into `TarGzExtractor` in `targz_extractor.go`; add `targz_extractor_test.go`
- [x] 1.4 Replace `download.go` dispatch logic in `Manager` with a `sources []Source` slice (iterated via `CanFetch`) and an `extractors []Extractor` slice (iterated via `CanExtract`); delete `download.go`
- [x] 1.5 Refactor `Manager.Request` to extract its core resolution logic into `Manager.Get(ctx context.Context, def ResourceDefinition, resourceID string) (string, error)`; update `Manager.Request(ctx, id string)` to look up the definition from the static spec by ID and delegate to `Manager.Get`
- [x] 1.6 Update `manager_test.go` to cover the refactored paths

## 2. Add file:// source support

- [x] 2.1 Implement `FileSource` in `file_source.go`: accept `file://` URIs and bare local paths; for both directories and archives resolve the path to an absolute value and return it as `redirectPath` without writing anything to the cache; the Manager's extractor pass reads from `redirectPath` directly for archives
- [x] 2.2 Prepend `FileSource` to the `sources` slice in `manager.go`
- [x] 2.3 Add `file_source_test.go` covering: directory returned as redirect path, archive returned as redirect path, non-existent path returns error, unsupported file type returns error
- [x] 2.4 Add `manager_test.go` integration tests: `file://` directory returned directly as redirect path, missing directory returns error, `file://` archive extracted into cache, `file://` archive always re-extracted (no checksum)

## 3. Add ZIP extraction support

- [x] 3.1 Implement `ZipExtractor` in `zip_extractor.go`; add `zip_extractor_test.go`
- [x] 3.2 Append `ZipExtractor` to the `extractors` slice in `manager.go`
- [x] 3.3 Add `manager_test.go` integration test for `.zip` archive extraction end-to-end

## 4. Add go-git dependency

- [x] 4.1 Add `github.com/go-git/go-git/v5` to `go.mod` and `go.sum`
- [x] 4.2 Verify the module builds cleanly after the dependency is added

## 5. Extend runtimeartifacts — git source support

- [x] 5.1 Add `IsGitSourceURL` helper (detects `git@`, `git://`, and `*.git` HTTPS/HTTP URLs) in `git_source.go`
- [x] 5.2 Implement `GitSource.CanFetch`: returns true for remote git URLs only; local paths (including local git repos) are handled by `FileSource` via `file://`
- [x] 5.3 Implement `GitSource.Fetch`: parse `@<ref>` suffix from the URL; resolve the ref via remote list-refs; shallow-clone on first fetch; hard-reset to the remote ref on subsequent fetches; support full commit SHAs — if no named ref points to the SHA, perform a full-depth clone and hard-reset to the target commit
- [x] 5.4 Implement SSH authentication for `git@` URLs via go-git's SSH agent auth; non-SSH URLs use no auth
- [x] 5.5 Update `ParseSpec` validator to require empty `Sha256` for git sources and non-empty `Sha256` for non-git sources
- [x] 5.6 Add `"any"` platform-key fallback to `ResourceDefinition.Resolve`
- [x] 5.7 Update `HttpSource.CanFetch` to exclude git URLs using `IsGitSourceURL`
- [x] 5.8 Prepend `GitSource` to the `sources` slice in `manager.go`
- [x] 5.9 Add `git_source_test.go` covering: `IsGitSourceURL`, `CanFetch` for remote URLs, SSH auth wiring, clone, update to new commit, branch/tag/SHA ref checkout, idempotent fetch on same commit

## 6. Library-level preset resolver

- [x] 6.1 Create `internal/deploy/preset_external.go` with `IsExternalPresetURI` (detects URI-scheme prefixes) and `ResolvePreset(ctx, manager, uri, presetType string) (string, error)` that constructs a `ResourceDefinition`, calls `manager.Get`, and verifies the preset manifest
- [x] 6.2 Implement `needsExtraction` to set `ResourceDefinition.Extract`: false for `file://` directories and git sources, true for archive URLs
- [x] 6.3 Validate `@<ref>` suffix in `ResolvePreset`: parse using `runtimeartifacts.ParseGitURL`; reject with an error when a ref is present but the stripped URL is not a git source
- [x] 6.4 After resolution, verify the resolved path contains the manifest file expected for the given `PresetType`; return a clear error if absent
- [x] 6.5 Add `preset_external_test.go` covering: `file://` directory resolved directly, missing manifest returns error, `@ref` on non-git URL returns error

## 7. Update CLI argument parsing

- [x] 7.1 Implement `resolvePresetRef` in `cmd/exasol/util.go`: plain names (no URI scheme, no path separators) return as `PresetRef{Name}`; everything else constructs a `Manager` and delegates to `ResolvePreset`, returning `PresetRef{Path}`
- [x] 7.2 Thread preset type (infrastructure/installation) from the argument's position in the parsed command line through `cmd/exasol/init.go` and `cmd/exasol/install.go` to `resolvePresetRef`
- [x] 7.3 Add `util_test.go` cases covering all input forms: plain name, relative path, absolute path, `file://`, `https://*.git`, `https://*.git@ref`, `http://*.git`, `git://`, `git@`, HTTP/HTTPS archive URL

## 8. User documentation

- [x] 8.1 Add a short "External presets" section to `README.md` under the existing presets content showing accepted source forms with one-line examples
- [x] 8.2 Create `doc/presets.md` with a dedicated "External preset sources" section covering: source classification rules, accepted schemes and URL patterns, `@ref` syntax and how named-ref resolution works, caching behaviour per source kind, and troubleshooting tips for common errors

## 9. Error messaging

- [x] 9.1 Update `internal/deploy/init.go` to include the list of available embedded preset names in the unknown-preset error message
- [x] 9.2 Add a helper to `internal/deploy/preset_defaults.go` to enumerate available embedded preset names
