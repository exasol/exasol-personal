## 1. Resource layer

- [x] 1.1 Allow `ResourcePath` on git sources with `Extract: false` in `ArtifactSpec.validate` ([internal/resource/spec.go](internal/resource/spec.go)).
- [x] 1.2 Apply `ResourcePath` in the non-extract branch of `Manager.resolveEntry` when the URL is a git source ([internal/resource/manager.go](internal/resource/manager.go)). Reuse `pathWithinRoot` for traversal safety.
- [x] 1.3 Add `ParsePresetURI(uri) (cleanURI, subpath string)` helper that strips a trailing `#<subpath>` fragment; keep `ParseGitURL` untouched.

## 2. Deploy layer wiring

- [x] 2.1 In `deploy.ResolvePreset`, call `ParsePresetURI` before checking `@ref`, populate `ArtifactSpec.ResourcePath` from the subpath, and pass the fragment-stripped URI onward ([internal/deploy/preset_external.go](internal/deploy/preset_external.go)).
- [x] 2.2 Reject a fragment on `file://` directory URIs with a clear error; accept it on git URLs and on archive URIs (`.tar.gz`/`.tgz`/`.zip`) across `http`/`https`/`file` schemes.
- [x] 2.3 Have `needsExtraction` operate on the fragment-stripped URI so an archive URL with a fragment is still detected as extractable.

## 3. Tests

- [x] 3.1 Unit tests for `ParsePresetURI`: bare URL, URL with only ref, URL with only fragment, URL with both, empty fragment, encoded `%23`.
- [x] 3.2 Resource unit tests: git source with `ResourcePath` returns the subdirectory path; git source with `..` in `ResourcePath` is rejected; different subpaths produce distinct cache entries; `ParseSpec` accepts git + `resource_path` + `extract: false`.
- [x] 3.3 Deploy-layer unit tests for `ResolvePreset` covering: `file://` directory + fragment errors; `file://` archive + fragment resolves the subpath; archive subdir missing the required manifest surfaces the existing "does not contain the expected … manifest" error. (Git-clone-plus-fragment is exercised at the resource layer via `resolveEntry`; a live git integration test is left for the higher-level suite because unit tests cannot spin up a git server.)
- [x] 3.4 CLI classification: no new test needed — `IsExternalPresetURI` inspects only the scheme prefix, so a trailing `#subpath` cannot affect classification; existing coverage still applies.

## 4. Documentation

- [x] 4.1 Update [doc/presets.md](doc/presets.md) — add "Selecting a preset from a subdirectory" subsection with fragment syntax and one example per supported source kind, plus new troubleshooting rows.
- [x] 4.2 Update [CONTEXT.md](CONTEXT.md) — remove the "must live at the repository root" caveat and describe the new syntax + precedence with `@ref`.
- [x] 4.3 Add a `CHANGELOG.md` entry under "Added".
- [x] 4.4 Update the README external-presets section with a note about the fragment.

## 5. Validation

- [x] 5.1 `go tool golangci-lint run ./...` and `go test ./...` pass locally (0 issues; all packages green).
- [ ] 5.2 OpenSpec validation for this change. _(Deferred: `openspec` CLI is not installed in this environment; run `openspec validate add-preset-subdirectory-selector --strict` before archival.)_
