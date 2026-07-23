## ADDED Requirements

### Requirement: Test suite is organized by test kind

The Exasol Personal Python test suite SHALL live under `tests/tests/` and be split into
exactly four kinds, each in its own directory: `integration/` (offline CLI, no cloud),
`deployment/` (provisioning/lifecycle), `e2e/` (connect/query/output workflows), and
`chaos/` (fault injection and recovery). No `testcases/` directory SHALL exist.

#### Scenario: Every test is discoverable by kind

- **WHEN** the suite is collected by pytest
- **THEN** each test resides under one of `integration/`, `deployment/`, `e2e/`, `chaos/`
- **AND** each test is stamped with the kind marker matching its directory
- **AND** no test resides under a `testcases/` path

#### Scenario: A whole kind can be selected by marker

- **WHEN** pytest is run with `-m e2e`, `-m deployment`, `-m chaos`, or `-m integration`
- **THEN** exactly the tests in the matching directory are selected

### Requirement: Cloud suites share one deployment without ordering hazards

The `deployment`, `e2e`, and `chaos` suites SHALL share a single session-scoped
`reusable_deployment` fixture defined in the root `conftest.py`, and every stateful cloud
test SHALL leave the deployment in a database-ready state on exit.

#### Scenario: Read-only e2e tests run against a running deployment regardless of order

- **WHEN** the cloud suites run in pytest's default cross-directory order
- **THEN** each stateful test (lifecycle, fault) restores the deployment to database-ready
- **AND** the read-only e2e connect/query tests observe a running deployment

### Requirement: PR and branch tests are represented in the suite

The net-new tests from PR #120 (`spot-30069`) and branch `harishe_v2.0.0-rc5` SHALL be
present in the four-kind suite, placed by kind, and deduplicated against existing tests, so
those sources can be abandoned without losing coverage.

#### Scenario: Offline tests run in the fast integration job

- **WHEN** a ported test carries the `launcher_tests` marker (needs no cloud)
- **THEN** it resides in `integration/` so the dir-scoped integration job runs it

#### Scenario: A mixed-marker source file is split by kind

- **WHEN** a source file contains both an offline test and a cloud test
- **THEN** the offline test lives in `integration/` and the cloud test in a cloud directory

#### Scenario: Duplicates are not re-introduced

- **WHEN** a source file also contains a test already present on `main`
- **THEN** that duplicate is dropped and each behavior remains tested exactly once

#### Scenario: Ported tests are grouped by area, not one file per test

- **WHEN** ported tests are added to the suite
- **THEN** they are grouped into area files (or merged into the existing same-area file)
- **AND** no test file contains a single test purely as a naming artifact

### Requirement: Tooling targets all cloud directories

The Taskfile cloud tasks SHALL run tests across the `deployment`, `e2e`, and `chaos`
directories so that marker-scoped selection is not silently limited to one directory.

#### Scenario: Marker-scoped task covers relocated tests

- **WHEN** a cloud task runs with `-m installation_e2e` or `-m infrastructure_e2e`
- **THEN** matching tests are collected from `deployment`, `e2e`, and `chaos` alike
