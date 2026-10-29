## ADDED Requirements

### Requirement: Interrupted deploy and destroy stop OpenTofu before the launcher exits

When `exasol deploy` or `exasol destroy` receives SIGINT or SIGTERM while
OpenTofu is running, the launcher SHALL stop OpenTofu, exit with an error only
after OpenTofu has exited, and record the deployment as interrupted during that
operation.

#### Scenario: The terminal interrupts a deploy or destroy

- **WHEN** the user presses Ctrl+C while `exasol deploy` or `exasol destroy`
  runs OpenTofu
- **THEN** the launcher exits with an error after OpenTofu has exited, and the
  deployment state records an interruption during that operation

#### Scenario: Only the launcher receives the signal

- **WHEN** only the launcher process receives SIGINT or SIGTERM while
  `exasol deploy` or `exasol destroy` runs OpenTofu
- **THEN** OpenTofu stops, the launcher exits with an error after OpenTofu has
  exited, and the deployment state records an interruption during that
  operation

#### Scenario: Status and info report the interruption

- **WHEN** a deploy or destroy was interrupted
- **THEN** `exasol status` reports status `interrupted` and names the
  interrupted operation, and `exasol info` reports deployment state
  `interrupted`
