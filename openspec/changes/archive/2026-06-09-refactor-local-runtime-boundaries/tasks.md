## 1. Local Runtime Package

- [x] 1.1 Create a dedicated local runtime package for runner paths, runner staging, VM command execution, SSH key/share preparation, runner state parsing, and cleanup.
- [x] 1.2 Move local runner state parsing and port validation tests into the local runtime package.
- [x] 1.3 Move runner staging, embedded runner writing, start/stop/destroy runtime behavior, and related tests into the local runtime package.
- [x] 1.4 Keep the local runtime package independent of deployment workflow state and backend selection.

## 2. Local Backend Adapter

- [x] 2.1 Refactor the local backend to call the local runtime package for deploy/start/stop/destroy operations.
- [x] 2.2 Keep deployment artifact writing in the local backend and map local runtime endpoint state into `config.DeploymentInfo`.
- [x] 2.3 Remove `nodes` from newly written local `deployment.json` files while preserving `connection` metadata for SQL, Admin UI, SSH, and shell support.
- [x] 2.4 Update local artifact generation tests to assert endpoint-based local metadata and no top-level `nodes` field.

## 3. Local Shell

- [x] 3.1 Add local SSH resolution from local connection metadata and the local runtime key path.
- [x] 3.2 Update local `shell host` and `shell container` to use local SSH resolution instead of node-derived SSH details.
- [x] 3.3 Preserve tofu/cloud shell behavior through existing node-derived SSH resolution.
- [x] 3.4 Add tests proving local shell metadata resolution works without `nodes`.

## 4. Compatibility and Verification

- [x] 4.1 Preserve config normalization and node-derived fallbacks for existing local and cloud deployment artifacts.
- [x] 4.2 Run Go formatting and focused tests for deploy, local runtime, config, and presets.
- [x] 4.3 Rebuild the launcher and run targeted local fake-runner integration coverage.
- [x] 4.4 Inspect a generated local `deployment.json` to confirm `nodes` is omitted and cloud preset outputs remain unchanged.
