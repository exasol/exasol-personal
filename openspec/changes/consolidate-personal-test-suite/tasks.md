## 1. Establish the four-kind layout

- [x] 1.1 Register kind markers (`integration`, `deployment`, `e2e`, `chaos`) in
      `tests/pyproject.toml`; keep the existing behavior markers.
- [x] 1.2 Stamp each test with its kind marker by directory in the root `conftest.py`
      (`pytest_collection_modifyitems`), so `-m e2e` / `-m chaos` select a whole kind.
- [x] 1.3 Create `tests/tests/e2e/` and `tests/tests/chaos/` packages; remove the empty
      `testcases/` directory and stale `__pycache__`.

## 2. Reorganize existing tests (no coverage change)

- [x] 2.1 Move the session-scoped `reusable_deployment` fixture to the root `conftest.py`
      so the deployment, e2e, and chaos suites share one cluster.
- [x] 2.2 Move read-only connect/query/output tests to `e2e/test_connect_query.py`
      (including the picklable `_connect_worker`); relocate `assets/people.csv` to `e2e/assets/`.
- [x] 2.3 Move the interrupt/recovery test to `chaos/test_lifecycle_faults.py`.
- [x] 2.4 Reduce `deployment/test_standard_deployment.py` to lifecycle + remote-archive and
      rename it `deployment/test_deploy_lifecycle.py`.
- [x] 2.5 Keep every stateful cloud test self-restoring to a database-ready state so
      read-only e2e tests are order-independent across directories.

## 3. Wire up tooling and docs

- [x] 3.1 Point the Taskfile cloud tasks (`tests-deployment`, `-infrastructure`,
      `-installation`, `-local`) at `deployment` + `e2e` + `chaos`.
- [x] 3.2 Update `tests/README.md` and `tests/testing.md` for the four-kind layout.
- [x] 3.3 Confirm SonarQube config (`tests/**/*.py` globs) needs no change.

## 4. Verification

- [x] 4.1 `ruff check` and `ruff format --check` clean on all changed/new files.
- [x] 4.2 `mypy` clean on all changed/new files.
- [x] 4.3 `pytest --collect-only` succeeds; `-m e2e|deployment|chaos|integration` and
      `-m installation_e2e|infrastructure_e2e` select the expected tests across directories.
- [ ] 4.4 Run the cloud suites against a real infra once (requires credentials) to confirm
      the shared-fixture split preserves ordering behavior end-to-end.

## 5. Port the actual tests from PR #120 and branch `harishe_v2.0.0-rc5`

- [x] 5.1 Port the 79 net-new branch `testcases/TC-*` tests into the four kinds by CI-safe
      rules (offline `launcher_tests` → `integration/`; cloud → `deployment`/`e2e`/`chaos`),
      adding a shared `tests/tests/testcase_helpers.py`. (62 files)
- [x] 5.2 Split the one mixed file (`test_tc_objstore_04`): offline help test stays in
      `integration/`, the cloud installed-versions test moves to `deployment/`.
- [x] 5.3 Port the 17 net-new PR #120 tests (10 files): offline → `integration/`, cloud
      read-only → `e2e/` (rewiring `e2e_deployment` → the shared `reusable_deployment` to
      avoid a second cluster), provisioning → `deployment/`, faults → `chaos/`.
- [x] 5.4 Drop duplicates that rode along (e.g. `test_remove_refuses_non_deployment_directory`,
      already in `test_reconfiguration.py`); register the `stress` marker.
- [x] 5.5 Full suite: ruff, ruff format, mypy, and `--strict-markers` collect all clean;
      zero duplicate test names.
- [ ] 5.6 Close PR #120 and abandon branch `harishe_v2.0.0-rc5` once merged.

## 6. Consolidate the ported tests (dedupe + regroup, no 1-file-per-test)

- [x] 6.1 Drop 6 ported tests that duplicate existing `main` tests: deploy refuse-preset,
      config-set-on-running, destroy --remove (vs `test_reconfiguration.py`); local
      memory/platform rejects (vs `test_install.py`); local port override (vs
      `test_local_deployment.py`).
- [x] 6.2 Collapse the ~62 one-test `test_tc_*.py` files into area files by kind:
      integration `test_backend/test_cache/test_connect_cli/test_preset_config/test_repo_hygiene`,
      deployment `test_object_storage/test_stackit/test_local_vm/test_deploy_ops`,
      chaos `test_faults`.
- [x] 6.3 Merge same-area ported tests into existing files: dir-resolution + info-states →
      `test_deployment_directory_resolution.py`; semantic version + version-check forwarding →
      `test_version_check.py`; help/inclusive-phrasing/powershell + unknown-flag →
      `test_cli.py`; non-writable-dir → `test_init.py`; connect contract tests + workflows →
      `e2e/test_connect_query.py`; reconcile + deploy-interrupt → `chaos/test_lifecycle_faults.py`.
- [x] 6.4 Add the missing `test_unknown_flag_exits_nonzero_with_usage` (was only in the PR's
      `integration/test_cli.py`, not ported initially).
- [x] 6.5 Remove the now-empty `test_tc_*` files and the shared helper module stays as
      `tests/tests/testcase_helpers.py`. Result: 25 test files, no one-test-per-file.
- [x] 6.6 Re-validate: ruff, ruff format, mypy (34 files), `--strict-markers` collect
      (186 selected / 6 provider-deselected), zero duplicate test names.

## Notes

Net-new *behavior* tests were not authored: the v2.1 checklist gaps (git presets, start/stop
`--json`, idempotency, blocked-state, piped SQL) are already covered by Go unit tests and the
existing Python `integration/test_external_presets.py`. Instead, the actual Python tests from
PR #120 and the branch were ported in so those two sources can be abandoned without losing
coverage. See `proposal.md`.
