## 1. Coverage Model

- [x] 1.1 Define the end-state fixture and annotation contract for capability, execution boundary, provider/platform applicability, state mutation, cost, and relevant product specification.
- [x] 1.2 Register the annotations and add focused tests for provider/platform validation and fixture-derived execution behavior.
- [x] 1.3 Add pytest summary output that reports selected and omitted boundaries, providers, platforms, specialized suites, and deployments created across workers.

## 2. Deployment Fixture Boundaries

- [x] 2.1 Introduce the controlled `deployment` fixture around the compiled launcher and migrate a representative fast deployment-state scenario with focused fixture tests.
- [x] 2.2 Introduce `local_deployment` as the explicit real-local boundary and migrate representative local lifecycle coverage.
- [x] 2.3 Split the shared live fixture into read-only `shared_live_deployment` and destructive or non-restorable `isolated_live_deployment` lifecycles with centralized cleanup and diagnostic preservation.
- [x] 2.4 Detect shared-live state contamination after each consumer and cover the clean and contaminated paths.
- [x] 2.5 Add an exclusive reusable-live boundary for vetted restorable mutations, with post-test restoration and quarantine on failure.

## 3. Capability-Oriented Test Organization

- [x] 3.1 Move controlled CLI, configuration, preset, deployment-directory, state, output, cache, and error-contract tests into capability-oriented fast coverage.
- [x] 3.2 Move live lifecycle and recovery scenarios into lifecycle-oriented coverage and give every mutating scenario an explicit reusable or isolated deployment boundary.
- [x] 3.3 Move read-only status, information, diagnostics, connection, and interaction scenarios into introspection or interaction coverage using the shared live boundary where valid.
- [x] 3.4 Establish narrow cloud and local smoke paths and remove comprehensive deployed-product assertions that exceed the repository testing boundary.
- [x] 3.5 Remove obsolete directories, directory-kind marker stamping, legacy selectors, and compatibility Task aliases as part of the completed cutover.

## 4. Provider, Platform, and CI Evidence

- [x] 4.1 Select live suites directly through pytest annotations and Task/CI configuration, retaining AWS broad coverage, focused AWS, Azure, and Exoscale smoke paths, and an explicit STACKIT omission until its deployment path works correctly.
- [x] 4.2 Preserve local release validation for every supported local host platform through compatible Linux AMD64 and macOS ARM64 runners.
- [x] 4.3 Publish selected and omitted test evidence in GitHub Actions summaries and cover matrix filtering and empty-selection failures.
- [x] 4.4 Verify that the normal pull-request workflow provisions no cloud resources and record its feedback duration against the roughly 15-minute target (29 seconds for the complete local launcher path).

## 5. Contributor Contract and Validation

- [x] 5.1 Align contributor testing documentation and Task commands with capability ownership, fixture vocabulary, live-test requirements, smoke boundaries, and OpenSpec evidence expectations.
- [x] 5.2 Validate that product-facing OpenSpec changes identify end-to-end evidence during review without introducing mandatory scenario identifiers or automated one-to-one enforcement.
- [ ] 5.3 Run formatting, linting, the fast end-to-end suite, focused live fixture tests, OpenSpec validation, and the full relevant repository checks.
- [ ] 5.4 Run AWS, Azure, Exoscale, and supported local smoke paths and verify cleanup evidence in CI.
