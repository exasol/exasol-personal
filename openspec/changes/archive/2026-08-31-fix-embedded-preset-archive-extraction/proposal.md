## Why

The runtime artifact cache's archive extraction only handles regular files and
symlinks in a way that depends on `os.Root` tolerating a directory entry's
trailing separator. On a Go toolchain in the affected range around go1.26.5,
it does not: a tar.gz archive containing an explicit directory entry, the
shape the glob-based-resource-groups embedder produces for embedded preset
archives, fails to extract. Because a failed member resolution is skipped
silently rather than reported, `exasol presets list` reports no presets, and
commands that resolve one, such as `exasol install local` and `exasol
presets export`, cannot find it, with nothing pointing at the cause.

## What Changes

- Archive extraction handles directory entries, so a tar.gz archive
  containing explicit directory entries extracts fully instead of failing.

## Capabilities

### Modified Capabilities

- `runtime-artifact-cache`: archive extraction covers directory entries in a
  tar.gz archive.

## Impact

- `internal/runtimeartifacts/targz_extractor.go`
- Any resource resolved through the embedded, extracted archive path, most
  notably infrastructure and installation presets.
