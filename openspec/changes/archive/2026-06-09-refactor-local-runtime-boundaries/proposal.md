## Why

The Exasol Local VM integration currently places runner lifecycle, local VM runtime paths, SSH setup, backend adaptation, and deployment artifact generation inside `internal/deploy`. This makes the deploy workflow package responsible for runtime implementation details and keeps the local `deployment.json` shaped around cloud nodes even though local deployments expose endpoints rather than nodes.

## What Changes

- Extract Exasol Local VM runner and runtime management out of `internal/deploy` into a dedicated runtime package.
- Keep deployment lifecycle orchestration in `internal/deploy`, with the local backend acting as a thin adapter from runtime state to launcher artifacts.
- Stop writing `nodes` for new local deployments; local artifacts will use top-level `connection` metadata for SQL, Admin UI, SSH, and shell support.
- Update local shell access to use local connection/runtime metadata instead of node-derived SSH metadata.
- Preserve cloud/tofu deployment artifacts and node-based behavior.
- Preserve compatibility with existing deployment artifacts that still contain `nodes`.

## Capabilities

### New Capabilities
- `local-runtime-boundaries`: Exasol Local runtime ownership, local endpoint artifact shape, and package boundary cleanup.

### Modified Capabilities

## Impact

- Local backend and runtime implementation.
- Local `deployment.json` shape for new deployments.
- Local shell host/container implementation.
- Tests for local artifact generation, shell connection metadata, and cloud artifact preservation.
- Package organization under `internal/`.
