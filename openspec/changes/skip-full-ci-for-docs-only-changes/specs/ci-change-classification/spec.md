## ADDED Requirements

### Requirement: Classify CI validation scope by changed file paths
The CI workflow SHALL classify each pull request run by comparing changed file paths against the repository `.ciignore` allow-list.

#### Scenario: Documentation-only changes are classified without full CI
- **WHEN** every changed file matches `.ciignore`
- **THEN** the workflow marks documentation quality checks as required
- **THEN** the workflow marks full CI as not required

#### Scenario: Mixed changes require full CI
- **WHEN** at least one changed file does not match `.ciignore`
- **THEN** the workflow marks full CI as required

#### Scenario: Empty change set is conservative
- **WHEN** the classifier cannot identify any changed files
- **THEN** the workflow marks full CI as required

### Requirement: Run documentation quality checks for documentation-only changes
The CI workflow SHALL run the documentation quality stage when `.ciignore` files changed in a pull request.

#### Scenario: Documentation files changed
- **WHEN** at least one changed file matches `.ciignore`
- **THEN** the documentation quality stage runs

#### Scenario: No documentation files changed
- **WHEN** no changed file matches `.ciignore`
- **THEN** the documentation quality stage is not required

### Requirement: Skip code validation only for documentation-only changes
The CI workflow SHALL skip Go linting, Python linting, build, unit test, and integration test jobs only when full CI is not required.

#### Scenario: Documentation-only change skips code validation
- **WHEN** the classifier marks full CI as not required
- **THEN** code validation jobs are skipped

#### Scenario: Non-documentation change runs code validation
- **WHEN** the classifier marks full CI as required
- **THEN** code validation jobs run

#### Scenario: Main branch push runs code validation
- **WHEN** the workflow runs for a push to `main`
- **THEN** code validation jobs run

### Requirement: Provide a final required CI gate
The CI workflow SHALL provide a final aggregate job that succeeds only when all validation stages required by the classifier have succeeded.

#### Scenario: Documentation-only validation succeeds
- **WHEN** full CI is not required
- **WHEN** documentation quality checks are required and succeed
- **THEN** the final aggregate job succeeds

#### Scenario: Full CI validation succeeds
- **WHEN** full CI is required
- **WHEN** all full CI jobs succeed
- **THEN** the final aggregate job succeeds

#### Scenario: Required validation fails
- **WHEN** any validation stage required by the classifier fails or is cancelled
- **THEN** the final aggregate job fails
