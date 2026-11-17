## Why

macOS CLI users expect launcher settings and durable data under the tool's home dotfolder. The filesystem layout is unreleased, so its defaults can be corrected directly before release.

## What Changes

- Store macOS deployments, SQL history and resource configuration under `~/.exasol/launcher`.
- Retain the native macOS cache root at `~/Library/Caches/exasol/launcher/resources`.
- Align tests, user documentation and the existing unreleased changelog entry with the final paths.
- Migrate only the released legacy layout; intermediate unreleased layouts are outside the migration contract.
- Deliver daily migration guidance and migration-code isolation in a separate follow-up change.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `deployment-directory-resolution`: macOS managed deployments use the home dotfolder.
- `sql-history-storage`: macOS SQL history uses the home dotfolder.
- `runtime-artifact-cache`: macOS resource configuration uses the home dotfolder.

## Impact

Tracked by SPOT-32510 under SPOT-33669. Affects `internal/launcherpaths`, launcher tests and path fixtures, published documentation and path specifications. PR #520 is already merged into the base. No new dependency or live infrastructure is needed.
