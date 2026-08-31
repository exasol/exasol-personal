## Context

The runtime artifact cache extracts tar.gz archives entry by entry
(`internal/runtimeartifacts/targz_extractor.go`). The glob-based-resource-
groups embedder is the first source that writes explicit directory entries
into an embedded archive; every other archive source used so far only
produced regular file and symlink entries, so the extractor's handling of
`tar.TypeDir` was never exercised against a real directory entry until
presets started going through it.

A tar directory entry's name carries a trailing slash by tar convention (for
example `aws/`). The extractor built its target path with
`filepath.FromSlash(hdr.Name)` and used it directly, so a directory entry's
target path kept that trailing separator. On a Go toolchain in the affected
range around go1.26.5 (not on go1.26.3, this project's declared floor, nor
on go1.27.0), `os.Root`'s `MkdirAll`/`Chmod` reject a path that still
carries that trailing separator, so creating the directory failed, and the
failure aborted the whole archive's extraction, since the extractor stops at
the first entry error. A user only needs a locally installed `go` at or
above the project's `go 1.26.3` floor for `GOTOOLCHAIN=auto` to select it, so
an affected patch release reached users without this project pinning one
itself.

Any resource embedded as a glob group, including every built-in
infrastructure and installation preset, is one such archive, so member
resolution failed for all of them on an affected toolchain. The failure
itself surfaced nowhere: `cmd/exasol/presets_catalog.go` reads each member's
manifest to build the preset list and silently skips a member whose manifest
cannot be read, so `exasol presets list` reported no presets instead of
failing loudly, even though the underlying resolution returned a hard
error.

## Goals / Non-Goals

**Goals:**

- A tar.gz archive containing explicit directory entries extracts
  completely, on every Go toolchain the project supports, regardless of
  which source produced the archive.

**Non-Goals:**

- Changing the ZIP extractor, which already creates directory entries
  through `os.MkdirAll` with a pre-cleaned path and was not affected.
- Any change to how glob-based-resource-groups builds its archives.
- Repairing a runtime artifact cache entry left behind by this defect before
  the fix.
- Surfacing a failed member resolution instead of silently skipping it in
  the preset catalog (`cmd/exasol/presets_catalog.go`). That silent-skip
  behavior is why this defect was invisible rather than a loud failure, but
  changing it is a separate concern from fixing extraction itself.

## Decisions

### Clean the entry path before using it, rather than special-casing directory entries

**Decision:** Build the target path as
`filepath.Clean(filepath.FromSlash(hdr.Name))` for every entry type, and
adjust the invalid-entry guard to reject a cleaned path of `.` or a bare
separator (the results Clean can produce for an entry naming the archive
root) instead of `""` or `.`.

**Rationale:** `filepath.Clean` is the standard way to normalize a path
before use, and applying it uniformly means a directory entry is handled by
the same code path as a file or symlink entry, rather than adding
directory-entry-specific trimming logic that the file and symlink branches
would not share. It also does not depend on which Go toolchain is running:
a cleaned path has no trailing separator to trip over on an affected
release, and behaves identically on a release that was never affected.

**Alternatives considered:** Trim a trailing separator only inside the
`tar.TypeDir` branch. Rejected: that would leave the general path-building
step un-normalized for every other entry type, so any future entry type with
an unusual name shape would need to rediscover the same fix.

## Risks / Trade-offs

- [A cache entry already populated by a prior, buggy extraction stays
  whatever partial or empty state that extraction left it in] → Not
  addressed by this change; a user affected by that state clears the
  runtime artifact cache to force re-extraction.
- [A future Go toolchain could reintroduce stricter trailing-separator
  handling in `os.Root`] → The fix no longer passes a trailing separator to
  any `os.Root` call, so it no longer depends on `os.Root` tolerating one.

## Migration Plan

No migration step is needed. The fix only changes how a future extraction
processes directory entries; it does not touch any already-cached artifact.
