## Why

The Python test suite historically drifted across three overlapping sources — PR #120
(`spot-30069`), branch `harishe_v2.0.0-rc5` (a 70-file `testcases/TC-*` layer), and the
v2.1 Confluence checklist. `main` has since converged: it already carries the deduplicated
`integration/` and `deployment/` suites and no longer contains `testcases/`, so those three
sources are redundant and can be abandoned.

Two findings from applying this change reshaped its scope:

1. **The "16 gap" behaviors are already covered.** The original gap analysis only inspected
   the Python sources and missed the Go unit-test layer, which is the primary test layer.
   Git-ref presets (`git_source_test.go`), start/stop `--json` (`deploymentControl_test.go`),
   start/stop idempotency and blocked-state guidance (`deploymentControl_test.go`,
   `status_test.go`, `reconfiguration_test.go`), piped/non-interactive SQL
   (`connect_test.go`, `shell_test.go`), and file/archive presets (Python
   `integration/test_external_presets.py`) are all tested. No net-new coverage is warranted.

2. **The only remaining work is organizational.** `main` used two Python test directories
   (`integration/`, `deployment/`) with behavior markers. This change reorganizes the cloud
   tests into a four-kind layout without adding or removing any test coverage.

## What Changes

- Introduce a four-directory test layout under `tests/tests/`: `integration/` (offline CLI),
  `deployment/` (provisioning/lifecycle), `e2e/` (connect/query/output workflows), and
  `chaos/` (fault injection/recovery). No `testcases/` directory.
- Split the mixed `deployment/test_standard_deployment.py`: read-only connect/query tests →
  `e2e/test_connect_query.py`; lifecycle + remote-archive → `deployment/test_deploy_lifecycle.py`;
  the interrupt/recovery test → `chaos/test_lifecycle_faults.py`.
- Move the shared session-scoped `reusable_deployment` fixture to the root `conftest.py` so
  the deployment, e2e, and chaos suites share one cluster; stamp each test with a kind marker
  (`integration`/`deployment`/`e2e`/`chaos`) by its directory.
- Preserve execution correctness: every stateful cloud test leaves the deployment
  database-ready on exit, so read-only e2e tests are safe under pytest's cross-directory order.
- Port the actual net-new Python tests from PR #120 (17 tests, 10 files) and branch
  `harishe_v2.0.0-rc5` (79 `testcases/TC-*` tests, 62 files) into the four kinds so both
  sources can be abandoned without losing coverage. Offline tests land in `integration/`;
  cloud tests in `deployment`/`e2e`/`chaos` by behavior. The branch's `testcases/helpers.py`
  becomes the shared `tests/tests/testcase_helpers.py`; PR read-only cloud tests are rewired
  from their own `e2e_deployment` fixture to the shared `reusable_deployment` (one cluster).
- Update the Taskfile cloud tasks and the testing docs to cover all three cloud directories.
- No product/runtime code changes.

## Capabilities

### New Capabilities

- `personal-test-suite`: the required organization of the Exasol Personal Python test suite
  (four kinds, one directory each, kind markers, a shared cloud deployment fixture, and the
  requirement that PR #120 and branch `harishe_v2.0.0-rc5` tests are represented in it).

## Impact

- `tests/tests/`: new `e2e/` and `chaos/` directories; `deployment/test_standard_deployment.py`
  split/renamed; `reusable_deployment` moved to the root `conftest.py`; `testcases/` removed.
- `tests/pyproject.toml`: four kind markers registered.
- `Taskfile.yml`: cloud test tasks target `deployment` + `e2e` + `chaos`.
- `tests/README.md`, `tests/testing.md`: documented layout updated.
- No change to `cmd/exasol` or `internal/`. SonarQube config already globs `tests/**/*.py`.
- PR #120 and branch `harishe_v2.0.0-rc5` are redundant with `main` and can be closed.
