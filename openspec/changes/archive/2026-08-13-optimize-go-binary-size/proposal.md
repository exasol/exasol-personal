## Why

Launcher downloads currently ship larger raw Go binaries than necessary. Reducing raw executable size lowers download and storage cost without changing end-user behavior.

## What Changes

- Apply low-effort Go build optimizations to Task and release builds.
- Document accepted and rejected binary-size techniques with their tradeoffs.
- Avoid higher-risk options such as executable packing, alternative compilers, and dependency replacement for size alone.
- No end-user CLI features are removed.

## Capabilities

### New Capabilities

- `binary-size-optimization`: Defines how launcher binary builds reduce raw executable size while preserving user-visible behavior.

### Modified Capabilities

None.

## Impact

- Build configuration in Task and GoReleaser.
- Development and release documentation.
- Raw launcher binaries for Linux, macOS, and Windows.
