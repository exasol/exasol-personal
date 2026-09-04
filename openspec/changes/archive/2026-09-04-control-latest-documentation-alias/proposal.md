## Why

Readers cannot tell which published version is current, because the version selector never shows
the `latest` alias. Publication also grants `latest` to every stable version, so publishing
documentation for an older maintenance release moves the alias and the site root backwards.

## What Changes

- Show the `latest` alias next to the version that carries it in the published version selector.
- Grant `latest` and the site root automatically only to the highest published stable version.
- Add a publication input that forces or withholds `latest` for a single publication.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `versioned-documentation-publishing`: Determine the `latest` alias from published version order
  and expose it to readers.

## Impact

The documentation configuration, the manual documentation workflow, its helper script and tests,
the CI guide, and the versioned documentation publishing specification change. Versions published
before this change keep their existing selector until they are republished. No runtime product
behavior or additional dependency is introduced.
