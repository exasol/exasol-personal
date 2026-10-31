Each numbered task below maps to exactly one commit. Every commit must pass the full
check suite (`task all`: unit tests, integration tests, lint, build) on its own, so
tasks are ordered and scoped so no intermediate commit leaves a location
half-migrated or a behavior claim untested.

## 1. Path Resolution Foundations

- [x] 1.1 Add one function per purpose (deployments root, history file, config
      file, cache root) to `internal/launcherpaths`, implemented per platform per
      the design's path table, alongside the existing flat-root functions and one
      function per legacy location; cover each with unit tests. Purely additive:
      no existing caller changes yet.

## 2. Cache Configuration Relocation

- [x] 2.1 Move the runtime-artifact cache's `RetentionDays` config to
      `resources.yaml` at the new configuration location, read and written by
      `internal/runtimeartifacts` itself. Cover with unit tests.

## 3. Cache Root Relocation

- [x] 3.1 Point the runtime-artifact cache root at the new platform-specific cache
      location under the `resources` leaf name. Cover with unit tests.

## 4. SQL History Relocation

- [x] 4.1 Point `internal/connect/shell.go`'s history path at the new history
      location. Create the history directory when the interactive shell starts,
      so a fresh install persists history too. Cover with unit tests.

## 5. Deployments Relocation

- [x] 5.1 Resolve the managed-deployments root and the default and named
      deployment directories at the new location in `internal/launcherpaths`,
      replacing the flat-root functions and the `internal/config` path
      functions. Update the integration tests that assert deployment locations,
      and point the launcher's platform configuration and cache directories at
      their temporary home so those tests stay hermetic.

## 6. Startup Wiring

- [x] 6.1 Once at CLI startup, before deployment-dependent flag pre-registration
      and before any command records an operation in progress, move each legacy
      deployment directory, the cache configuration, and the SQL history to its
      new location with one atomic rename that refuses an existing destination,
      and delete the legacy cache. Refuse (with a clear error naming both paths)
      any deployment name that already exists at the new location while still
      migrating the other deployments in the same run. Run without prompting for
      confirmation, exit before command dispatch on failure, and write a
      completion marker only after full success. First refuse while a legacy
      deployment is locked or a legacy local deployment runs, letting `stop` and
      `diag unlock` through. Skip migration for help and shell completion. Remove
      the legacy directories left empty. Report what changed on standard error
      before the requested command's own output. Cover it with unit tests,
      including the collision case, and with launcher tests for the migrated
      command, a failed migration, and the history file.

## 7. Documentation

- [x] 7.1 Update command help text that names the legacy path. Already satisfied:
      help text builds its paths through `defaultDeploymentDirDisplayPath()` /
      `deploymentsRootDisplayPath()`, which already resolve through the
      `internal/launcherpaths` functions from section 5. Verified by running
      `exasol --help` with a fresh HOME. No code change; no commit for this item.
- [x] 7.2 Document every launcher file location and the upgrade migration in
      `user-docs/content/manage-deployments.md`, and link it from the cache page.
- [x] 7.3 Add a CHANGELOG entry under `Unreleased`.
