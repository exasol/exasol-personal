# Proposal: A Simple Python End-to-End Test Architecture

**Jira:** SPOT-32218
**Status:** Draft for architectural review

## Summary

Replace the current mix of test directories, overlapping pytest markers, and the proposed TOML inventory with one simpler model: tests describe themselves, fixtures define their execution boundary, and CI decides when and where they run.

The refactor is a full cutover to the new model. It does not preserve a migration architecture or compatibility layer.

## Problem

The current suite expresses similar information through directory names such as `integration`, `deployment`, `e2e`, and `chaos`, markers such as `installation_e2e` and `infrastructure_e2e`, provider markers, fixture choice, and CI configuration. A contributor must understand how all of these mechanisms interact before knowing where a test belongs or how it runs.

A separate TOML test inventory would move this complexity into another file. It would also create a second source of truth that can drift away from the tests.

The architecture should answer four questions with four clear mechanisms:

| Question | Owner |
|---|---|
| What behavior does the test cover? | Test name, location, and OpenSpec annotation |
| What resources may the test use or modify? | Fixture |
| Which exceptional properties affect selection? | Minimal pytest annotations |
| Which environments run which suites? | CI and task configuration |

## Design principles

1. Test metadata lives with the test.
2. Fixtures own setup, teardown, and resource-sharing rules.
3. Markers describe cross-cutting facts, not directory categories.
4. Provider scheduling is CI policy, not duplicated in a test inventory.
5. OpenSpec traceability is capability-level evidence, not a claim of complete specification coverage.
6. The default test suite is fast and does not provision real infrastructure.

## Proposed architecture

### Test organization

Use two broad execution areas:

- **Launcher tests** exercise the real launcher with controlled inputs and no provisioned infrastructure. They form the fast default suite.
- **Live tests** exercise a real deployment and are grouped by product capability, such as lifecycle, introspection, interaction, and local runtime behavior.

Recovery and fault tests remain beside the capability they protect. They do not need a separate `chaos` taxonomy. Smoke tests also remain beside their capability instead of being copied into provider-specific directories.

The existing `integration`, `deployment`, `e2e`, and `chaos` classifications are removed during the refactor.

### Execution boundaries through fixtures

Five fixtures provide the vocabulary needed by the suite:

| Fixture | Meaning |
|---|---|
| `deployment` | Controlled launcher execution without real provisioned resources |
| `shared_live_deployment` | Reusable real deployment for read-only tests |
| `reusable_live_deployment` | Exclusive reusable deployment for vetted mutations that restore a ready database |
| `isolated_live_deployment` | Test-owned real deployment for destructive or non-restorable tests |
| `local_deployment` | Real local runtime with local-specific lifecycle management |

The fixture selected by a test is authoritative for resource ownership. A marker must not silently change whether a deployment is shared, reusable, or isolated.

Tests with exceptional setup requirements, such as interrupted provisioning, custom sizing, historical-launcher handoff, or a module-scoped local journey, may define a narrowly named test-local fixture backed by the deployment manager. These exceptions retain explicit ownership and centralized cleanup without expanding the primary fixture vocabulary.

### Minimal annotations

Keep only annotations that provide information unavailable from the test location or fixture:

| Annotation | Meaning |
|---|---|
| `openspec(*capabilities)` | Links the test to one or more OpenSpec capability IDs |
| `smoke` | Marks a critical-path test selected by limited provider jobs |
| `providers(*names)` | Restricts a test that cannot run on every applicable provider |
| `platform(*names)` | Restricts a test to run on a certain environment or platform. Ex. Windows arm64 |
| `chaos` | Highlights disruptive recovery or fault behavior |
| `stress` | Identifies intentionally expensive stability or load coverage |

No `providers` annotation means the test is provider-neutral within its execution boundary. Provider annotations are exceptions, not a mandatory list on every live test.

Standard pytest annotations such as `skipif` and `parametrize` remain available for their normal purposes.

The old kind, environment, and individual provider markers are removed. In particular, `installation_e2e`, `infrastructure_e2e`, `local_e2e`, `launcher_tests`, and `provider_*` no longer define the architecture.

Example module-level metadata for a cohesive capability:

```python
pytestmark = pytest.mark.openspec("runtime-artifact-cache")


def test_cache_list_json_is_valid(deployment: Deployment) -> None:
    ...
```

Example metadata for a critical live test:

```python
@pytest.mark.openspec("deployment-lifecycle-recovery")
@pytest.mark.smoke
def test_stop_and_start(reusable_live_deployment: Deployment) -> None:
    ...
```

Example provider restriction:

```python
@pytest.mark.providers("aws", "azure")
def test_remote_archive_registered(
    shared_live_deployment: Deployment,
) -> None:
    ...
```

### OpenSpec traceability

The `openspec` annotation contains stable capability directory IDs, for example `connect-sql-input` or `deployment-info-reporting`. Module-level annotations are preferred when all tests in a module provide evidence for the same capability. Test-level annotations handle exceptions or tests covering multiple capabilities.

The first version registers and exposes this annotation without validating capability existence during collection. This keeps ordinary test execution independent from the specification directory while preserving metadata for future traceability tooling.

An on-demand report can invert the collected annotations to show capabilities and their associated tests. The report is generated from pytest collection and is not checked into version control.

This first version deliberately does not introduce scenario IDs, coverage percentages, one-to-one mappings, or a completeness gate. An annotation means that a test provides relevant evidence for a capability. It does not mean every scenario in that capability is covered.

### Selection and CI policy

Task commands provide a small user-facing interface:

| Intent | Selection |
|---|---|
| Fast feedback | Launcher tests |
| Full live validation | All applicable live tests for one provider |
| Provider smoke validation | Applicable live tests marked `smoke` |
| Local validation | Tests using the local execution boundary |
| Recovery validation | Applicable tests marked `chaos` |

CI owns provider frequency and breadth:

| Environment | Coverage |
|---|---|
| AWS | Broad live suite |
| Azure | Smoke suite |
| Exoscale | Smoke suite |
| Local Linux AMD64 | Local suite |
| Local macOS ARM64 | Local suite |
| Stackit | Omitted while the provider integration is not working correctly |

Every CI smoke job must fail when it collects zero tests. This protects the scheduled environment without coupling pytest to the complete CI matrix.

There is intentionally no global rule that every smoke test must be selected by an active environment. Such a rule would duplicate CI policy inside the test framework and would make temporary provider omissions, including Stackit, appear to be test metadata errors.

Test summaries should state the selected execution area, provider, and relevant annotations. CI should also state that Stackit coverage is currently omitted so the gap remains visible.

## Representative effect on current tests

The refactor changes structure and metadata without requiring an exhaustive inventory:

| Current example | Proposed expression |
|---|---|
| Cache command tests | Launcher tests with a module-level `openspec("runtime-artifact-cache")` annotation |
| Reconfiguration tests | Launcher tests with `openspec("deployment-reconfiguration")` |
| `test_stop_and_start` | Live lifecycle test using the reusable fixture and marked `smoke` |
| `test_info_includes_connection_details` | Live introspection test using the shared fixture and linked to `deployment-info-reporting` |
| Lifecycle interruption and recovery tests | Live lifecycle tests using reusable or isolated fixtures according to restoration behavior and marked `chaos` |
| SLC tests | Local live tests linked to `local-slc-management` |
| Connect and query tests | Live interaction tests using shared, reusable, or isolated fixtures according to mutation and restoration behavior |

This is enough metadata to understand and select the tests. A separate row for every test would recreate the rejected inventory in Markdown.

## Validation

The framework validates only local facts it owns:

- annotation syntax;
- known provider names;
- valid fixture and annotation combinations where unsafe sharing could occur;
- non-empty collection for each CI job.

It does not validate provider scheduling policy from inside pytest.

## Full-cutover outcome

At completion:

- all tests use the new organization, fixtures, and minimal annotations;
- the TOML inventory and its loader, schema, and generated profiles do not exist;
- old architectural markers and compatibility aliases are removed;
- task commands and CI select the new model directly;
- documentation describes only the resulting framework;
- Stackit remains visibly omitted until its integration works again.

## Non-goals

- Building a custom test-management system
- Adding a dependency for metadata or report generation
- Maintaining an external test inventory
- Annotating every test with every environment it may run in
- Running every live test against every provider
- Proving complete OpenSpec scenario coverage in the first version
- Preserving the old framework during a staged migration

## Success criteria

The proposal succeeds when:

1. A contributor can place and annotate a new test without consulting a separate inventory.
2. The fixture alone makes resource ownership and isolation clear.
3. Fast tests remain the default and cannot provision live infrastructure.
4. Provider smoke jobs cannot pass after selecting no tests.
5. OpenSpec capability-to-test evidence is available from pytest collection for future reporting.
6. CI output clearly reports provider coverage and the temporary Stackit omission.
7. The old directories, markers, and TOML design are fully removed.

## Decision requested

Approve the annotation-and-fixture model as the target architecture for SPOT-32218 and use it as the basis for the full test framework refactor.
