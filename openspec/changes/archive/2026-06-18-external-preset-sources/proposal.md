## Why

Infrastructure and installation presets are currently limited to embedded presets (shipped inside the binary) or bare filesystem paths, making it impossible for users or teams to share and reuse custom preset definitions without modifying the binary or manually distributing files. Supporting external sources (git repos, remote archives, local directories via URI) enables ecosystem extensibility and easier preset authoring workflows.

## What Changes

- Preset resolution now accepts additional source kinds alongside the existing embedded name lookup:
  - A **git repository URL** (e.g. `https://github.com/org/my-presets.git`, `git@github.com:org/my-presets.git`, or `git://github.com/org/my-presets.git`), with an optional `@<ref>` suffix to pin a branch, tag, or commit
  - A **`file://` URI** pointing to an existing local directory containing a preset
  - An **`https://` or `http://` URL** pointing to a downloadable archive (`.tar.gz`, `.zip`)
- Embedded presets remain the default and **always take priority** over external sources when the same name exists.
- An input is treated as an **external source** when it begins with a recognised URI scheme (`file://`, `https://`, `http://`, `git://`, or `git@`) — bare names continue to resolve against embedded assets first.
- External preset sources are resolved to a local path before the rest of the launcher sees them; no existing preset loading code changes.
- The `runtimeartifacts` library is refactored around `Source` and `Extractor` interfaces, then extended with ZIP extraction, git source support, file:// pass-through, and a `Manager.Get` API so that preset definitions constructed at runtime can be resolved through the same caching infrastructure used for tool binaries.

## Capabilities

### New Capabilities

- `external-preset-resolution`: Parse and validate preset identifiers that carry a URI scheme; classify an input as embedded vs. external; implement the resolution priority rule (embedded wins unless scheme present); propagate preset type (infrastructure/installation) so the correct manifest is verified after fetching.
- `preset-source-fetching`: Resolve preset content from a git repository (cached by commit hash), a remote archive URL (cached when a checksum is provided; always re-fetched otherwise), or a local `file://` directory (symlinked from the cache on first access).

### Modified Capabilities

- none

## Impact

- `internal/runtimeartifacts`: refactored around `Source` and `Extractor` interfaces (`HttpSource`, `FileSource`, `GitSource`, `TarGzExtractor`, `ZipExtractor`); extended with `Manager.Get(ctx, def, resourceID)` for ad-hoc definitions, git source support, ZIP extraction, `file://` symlink pass-through, optional checksum for runtime-constructed specs, and the `"any"` platform key fallback.
- `internal/deploy`: `preset_external.go` added — `IsExternalPresetURI` classifier and `ResolvePreset` resolver; no changes to existing preset loading code.
- `internal/presets`: no changes — `PresetRef` is unchanged; external URIs are resolved to a local path before `PresetRef` is constructed.
- `cmd/exasol`: CLI argument parsing detects URI schemes and delegates to the library-level preset resolver; result is a `PresetRef{Path}` identical to a user-supplied filesystem path.
- New dependency: `github.com/go-git/go-git/v5` for pure-Go git operations (no external `git` binary required).
