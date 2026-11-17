## 1. Final macOS paths

- [x] 1.1 Resolve macOS durable data and resource configuration under the home dotfolder; update exact-path Go coverage, Python path fixtures, published path examples and the existing unreleased changelog entry together. Commit: `fix: Use launcher dotfolder for macOS data and settings`.

## 2. Validation and review

- [x] 2.1 Run focused launcher-path and startup migration Go tests; run the launcher migration, listing and selection tests with a rebuilt binary. Verify CLI help and resource configuration reporting against a temporary home. Record native versus cross-platform evidence accurately. Validation only; no commit.
- [x] 2.2 Run repository-required formatting, lint and `task all`, plus affected documentation checks; cross-compile affected packages for macOS and Windows if native runs are unavailable. Fix findings in task 1.1's commit. Validation only; no commit.
- [x] 2.3 Obtain independent code review and evaluate the affected CLI/documentation behavior, using temporary homes and no live infrastructure. Resolve findings in task 1.1's commit or record a disposition. Validation only; no commit.

## Delivery sequence

1. Sync and archive this completed change after validation; commit only the resulting main specifications and archived OpenSpec artifacts. Commit: `docs(openspec): Record final macOS launcher paths`.
2. Prepare the PR against latest main, reference SPOT-32510 and completed verification, and publish through the delivery workflow. Keep the reminder change in its own stacked PR. Delivery only; no commit.

## Verification evidence

- `task fmt` and `task all` passed on Linux AMD64. The full run included lint, strict documentation build, CLI reference verification, Go race tests and 284 passing launcher tests (7 skipped).
- Focused path/startup migration Go tests passed. The changed package and tests cross-compiled for macOS ARM64 and Windows AMD64. Native macOS/Windows execution was unavailable.
- Independent review found no confirmed defects in the final tracked diff. The CLI-only evaluation found no supported defects in the changed path, migration, redirection and collision-recovery behavior.
- Launcher test evidence: `test_launcher_path_migration.py`, `test_deployments_list.py`, and `test_cache.py::test_cache_paths_and_legacy_configuration_use_platform_locations`, with the corresponding capability markers.
- All three changed main specifications passed strict validation after sync. Archive required wording-preserving paragraph unwrapping in two existing deployment-resolution requirements. Three unrelated main specifications retain their pre-existing validator errors.
