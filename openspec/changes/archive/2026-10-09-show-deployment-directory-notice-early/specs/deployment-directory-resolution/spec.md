## MODIFIED Requirements

### Requirement: CLI SHALL keep the resolved deployment directory visible
Commands that resolve to the default deployment directory or to a named deployment directory SHALL emit one clear human-facing message containing the resolved path on standard error before command-specific execution begins.

#### Scenario: Default directory message is shown early
- **WHEN** a deployment command resolves to the default deployment directory
- **THEN** the user sees a message on standard error containing the resolved default deployment directory path before command-specific execution begins
- **AND** the message does not appear again after command-specific execution ends

#### Scenario: Named directory message is shown early
- **WHEN** a deployment command resolves to a named deployment directory via `--deployment <name>`
- **THEN** the user sees a message on standard error containing `<name>` and the resolved path before command-specific execution begins
- **AND** the message does not appear again after command-specific execution ends

#### Scenario: JSON payload output remains parseable
- **WHEN** a deployment command produces JSON output and resolves to the default deployment directory or a named deployment directory
- **THEN** the command's stdout remains valid JSON
- **AND** the human-facing resolved-directory message is emitted on standard error, not stdout
