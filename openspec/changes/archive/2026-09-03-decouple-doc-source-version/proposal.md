## Why

Release tags are immutable and older releases may need documentation published after their tags were created. Publication therefore needs to select historical content independently from the version shown to readers.

## What Changes

- Replace the single publication target with an immutable source revision and an independently selectable documentation version.
- Derive the documentation version from conventional version tags when no explicit version is supplied.
- Treat the selected revision as a self-contained documentation snapshot with its content and publishing environment.
- Keep deletion version-based and preserve the existing version catalog behavior.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `versioned-documentation-publishing`: Select immutable source content independently from its published semantic version.

## Impact

The manual documentation workflow, its local helper scripts and tests, the CI guide, and the versioned documentation publishing specification change. No runtime product behavior or additional dependency is introduced.
