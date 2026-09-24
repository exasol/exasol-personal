## Why

Exasol Personal's Python black-box tests use overlapping suite names, markers, and deployment lifetimes that obscure what behavior they prove, which infrastructure they require, and how much feedback they provide. The suite needs an explicit contract now so contributors can add reliable coverage while maintainers keep pull-request latency and live-deployment cost visible.

## What Changes

- Organize Python end-to-end coverage around product capabilities and user journeys, with execution boundaries expressed independently from behavior ownership.
- Make the normal pull-request suite exercise the compiled launcher without cloud provisioning and target useful feedback within roughly 15 minutes.
- Give live tests explicit shared, reusable, or isolated deployment ownership according to whether mutations can be safely restored.
- Limit chaos coverage to deliberately abnormal external failures.
- Keep test metadata on tests through pytest annotations and fixtures, while Task and CI configuration define provider and platform coverage for AWS, Azure, and Exoscale plus local release validation on every supported host platform. Record STACKIT as omitted until its deployment path works correctly.
- Report selected and omitted environments, providers, and specialized suites so a successful partial run cannot be mistaken for complete validation.
- Preserve OpenSpec-to-end-to-end evidence as a testing principle while deferring mandatory one-to-one mappings and automated enforcement.
- Replace the legacy test taxonomy and selection contract in one completed cutover without retaining compatibility selectors in the finished repository.
- Align contributor documentation, task commands, pytest selection, and CI with the resulting contract.

## Capabilities

### New Capabilities

- `end-to-end-test-workflow`: Defines observable contributor and CI behavior for selecting fast, live, provider, platform, and specialized Python end-to-end evidence and reporting omitted coverage.

### Modified Capabilities

None.

## Impact

- Python test organization, fixtures, markers, and framework helpers under `tests/`.
- Test-related Task commands and GitHub Actions workflows.
- Contributor testing documentation.
- Existing product behavior and the pytest framework remain unchanged.
