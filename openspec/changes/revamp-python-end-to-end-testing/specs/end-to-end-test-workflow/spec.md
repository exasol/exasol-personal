## ADDED Requirements

### Requirement: Pull-request end-to-end validation is cloud-free
The project SHALL provide a normal pull-request validation path that exercises the compiled launcher through supported external interfaces without requiring cloud credentials or provisioning cloud resources, and the path SHALL target useful feedback within roughly 15 minutes.

#### Scenario: Contributor validates a pull request
- **WHEN** a contributor runs or triggers the normal pull-request validation path
- **THEN** the compiled launcher is exercised without cloud credentials or cloud provisioning
- **AND** live deployment evidence is reported as omitted

### Requirement: Live evidence is explicitly selectable
The project SHALL provide explicit selection of live behavior, local deployment, cloud smoke, and specialized recovery evidence independently from the normal pull-request path.

#### Scenario: Maintainer selects live behavior evidence
- **WHEN** a maintainer selects a live behavior suite and a supported provider
- **THEN** the requested suite runs against that provider
- **AND** the result identifies the selected suite and provider

#### Scenario: Maintainer selects specialized recovery evidence
- **WHEN** a maintainer selects chaos recovery coverage
- **THEN** only scenarios for deliberately abnormal external failures are included as specialized recovery evidence

### Requirement: Tests expose their execution contract locally
Python end-to-end tests SHALL expose their execution boundary through their requested fixture and SHALL expose exceptional selection facts through pytest annotations on the tests themselves.

#### Scenario: Contributor inspects or collects a test
- **WHEN** a contributor inspects or collects a Python end-to-end test
- **THEN** the requested fixture identifies whether execution is controlled, local, shared live, reusable live, or isolated live
- **AND** any smoke, provider, platform, chaos, stress, or OpenSpec metadata is available from pytest collection without a separate inventory

### Requirement: Validated deployment targets have live smoke evidence
The release-validation workflow SHALL provide clearly identified live smoke paths for AWS, Azure, and Exoscale and SHALL provide local deployment validation for every supported local host platform. It SHALL identify STACKIT as omitted until its deployment path works correctly.

#### Scenario: Cloud release evidence is selected
- **WHEN** release validation selects cloud smoke evidence
- **THEN** AWS, Azure, and Exoscale each have a selectable path covering provisioning, installation, basic interaction, lifecycle, and destruction

#### Scenario: STACKIT coverage is omitted
- **WHEN** test evidence is summarized while the STACKIT deployment path does not work correctly
- **THEN** STACKIT is reported as omitted rather than validated
- **AND** the reason for the omission is visible

#### Scenario: Local release evidence is selected
- **WHEN** release validation selects all local deployment evidence
- **THEN** every supported local host platform is represented by a compatible runner and smoke path

### Requirement: Test results identify omitted evidence
Local and CI test summaries SHALL identify the selected execution boundaries, providers, platforms, and specialized suites, SHALL identify applicable evidence that was omitted, and SHALL report the number of deployments created during the run.

#### Scenario: Fast validation succeeds without live tests
- **WHEN** the cloud-free validation path succeeds without selecting live tests
- **THEN** its summary reports the fast evidence as successful
- **AND** reports live environments and providers as omitted

#### Scenario: A filtered live matrix succeeds
- **WHEN** a live workflow runs only a subset of supported providers or platforms
- **THEN** its summary identifies both the selected targets and the omitted supported targets

#### Scenario: Concurrent deployment tests complete
- **WHEN** deployment tests run across one or more workers
- **THEN** the final summary reports the total number of deployments created across all workers

### Requirement: Product specifications have end-to-end evidence
Changes to normative product-facing OpenSpec scenarios SHALL normally identify corresponding automated end-to-end evidence, while internal-only specification constraints MAY use other verification.

#### Scenario: Product-facing behavior changes
- **WHEN** a change adds or modifies a normative product-facing OpenSpec scenario
- **THEN** the change identifies added, updated, or existing automated end-to-end evidence for that scenario

#### Scenario: Specification describes an internal constraint
- **WHEN** an OpenSpec scenario is classified as an internal implementation constraint
- **THEN** the change may identify focused implementation-level verification instead of end-to-end evidence
