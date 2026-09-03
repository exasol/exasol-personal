# Design — Preset subdirectory selector

## Context

Preset URIs on the CLI today:

```
[<scheme>://…|git@…][@<ref>]
```

Git clones and archive extractions produce a directory tree. The manifest for the preset
(`infrastructure.yaml` / `installation.yaml`) must live at the resolved root. That constraint
prevents users from bundling multiple presets in one repository or archive.

## Decisions

### D1. Fragment (`#subdir`) syntax for the subpath

Alternatives considered:

- **Fragment `#subdir`** (chosen). Standard Web/Go/pip/npm idiom, composes with the existing
  `@<ref>` suffix without grammar changes, shortest to type.
- **Terraform-style `//subdir`**. Familiar to Tofu users but requires reworking the ref
  grammar (moving to query params or overloading `@ref`) to avoid ambiguity with the leading
  scheme `://`.
- **Query parameter `?path=…`**. Composable but longest form and shifts `@ref` for
  consistency.

Rationale: the fragment is orthogonal to `@ref`, so the existing `ParseGitURL` behaviour is
untouched and the added parser is a single split-on-last-`#` step.

### D2. Fragment parsing is a preset-level concern

Alternatives considered:

- Extend `resource.ParseGitURL` to return `(repoURL, ref, subpath)` and update every
  call site.
- **Add `resource.ParsePresetURI(uri) (cleanURI, subpath string)`** (chosen), called
  by `deploy.ResolvePreset` before it hands the URL to the manager.

Rationale: the subpath applies equally to non-git archive sources, so tying it to
`ParseGitURL` would either force a git-specific helper into unrelated call sites or add a
second parser. A single helper on the resource package keeps the concept in one
place. `ParseGitURL` continues to see a URL with no fragment, so ref parsing is unchanged.

### D3. Scope covers git and archive sources; `file://` directories still reject the fragment

- Git URLs (any supported git scheme).
- Remote archives (`http://` / `https://` `.tar.gz` / `.tgz` / `.zip`).
- Local archives (`file://` `.tar.gz` / `.tgz` / `.zip`).
- **Excluded**: `file://` directories. Users can pass the subdirectory path directly.
  Accepting a fragment there would create two ways to express the same thing.

Rationale: archives already support `ResourcePath` via `Extract: true`, so the marginal cost
of extending fragment support to them is zero once the parser exists.

### D4. Cache identity — accept duplication

`artifactIdentity` already hashes `ResourcePath` into the cache key. Two requests for the
same repo/ref but different subpaths produce two distinct cache entries, each with its own
git clone.

Alternatives considered:

- Share the clone across subpath variants by splitting the fetch key `(URL, ref)` from the
  resolved-path key `(URL, ref, subpath)`. This is a `resolveEntry` and index-schema
  refactor.
- **Accept the duplication** (chosen). Clones are shallow and small; monorepo preset users
  do not typically install many subpaths from the same repo in one launcher install. The
  refactor can happen later without breaking the URI syntax.

### D5. Resource validation loosening is git-only

`ArtifactSpec.validate` currently rejects `ResourcePath` when `Extract: false`. That guard
protected against nonsensical archive-less configs. It remains in force for non-git,
non-extract sources (bare HTTP files, `file://` bare files), which are meaningless with a
subpath. For git sources, cloning already produces a browsable tree, so the guard is
relaxed.

### D6. Preset identity persistence is unchanged

`presetIdentityOf` records external presets as `path:<absolute-cache-path>` after
resolution. With a subpath, the path naturally points at the resolved subdirectory, so the
existing "preset must match on re-init" check works unchanged and no state-file migration is
needed.

## Risks

- **Shell escaping**: `#` starts a comment in interactive shells. Documented; users must
  quote the URL. `zsh` / `bash` both handle it inside double quotes.
- **URL encoding**: `%23` in a subpath must be decoded, and `%2F` tolerated. The parser
  splits on the *literal* `#` (URL fragments are always trailing); percent-decoding of the
  subpath happens via `net/url` where possible.
- **`@` inside subpath**: Fragment is stripped before `ParseGitURL` runs, so an `@` inside
  the subpath cannot confuse ref parsing.
- **Path traversal**: `pathWithinRoot` (already used for archive `ResourcePath`) rejects
  `..` segments. Reused for the git case.

## Migration

None. The feature is additive; no existing external preset URI contains `#`.
