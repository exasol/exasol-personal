## Why

Startup migration keeps a legacy deployment at its original location while it
is in use or cannot be moved. Named and implicit default selection currently
resolve to the new location, so users cannot reach that deployment by its name
or through the default selection.

## What Changes

- Resolve a named deployment at the new managed location when it exists.
  Otherwise, use the matching legacy deployment when it exists.
- Apply the same location preference to implicit default selection. When
  neither location exists, select the new location for initialization.
- Warn when selection uses a legacy path, and use the selected path when
  reading deployment options before command execution.
- List a legacy managed deployment when its name has no entry at the new
  location. List the new entry when both locations have that name.
- Explain the fallback in user documentation and the changelog.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `deployment-directory-resolution`: Define location preference for named and
  implicit default selection while a deployment remains at the legacy location.
- `deployment-directory-listing`: Include kept legacy deployments according to
  the same name preference used by deployment selection.

## Impact

- Deployment directory resolution, early deployment option lookup, and managed
  deployment listing in `cmd/exasol`.
- Go and launcher tests, user documentation, and the changelog.
