## ADDED Requirements

### Requirement: Automatic launcher update checks SHALL follow successful commands
For eligible commands, the launcher SHALL perform the silent update check only after the command completes successfully. An available-update hint SHALL appear as final call-to-action guidance, subject to the existing JSON suppression rule.

#### Scenario: Successful command completes before update guidance
- **WHEN** an eligible command completes successfully and a newer launcher version is available
- **THEN** the automatic update check runs after the command has completed
- **AND** its update hint appears after the command's notices and primary result

#### Scenario: Interactive session receives update guidance on exit
- **WHEN** an interactive command starts a session and a newer launcher version is available
- **THEN** the automatic update check and any update hint occur after that session exits successfully

#### Scenario: Failed command does not check for updates
- **WHEN** an eligible command fails
- **THEN** the launcher does not perform the automatic update check for that invocation
- **AND** no automatic update hint is emitted

#### Scenario: Removed deployment state is skipped safely
- **WHEN** a successful `exasol remove` or `exasol destroy --remove` invocation removes the selected deployment directory
- **THEN** the launcher skips the automatic update check
- **AND** the removed directory is not recreated
- **AND** no automatic update hint is emitted

#### Scenario: Version command remains exempt
- **WHEN** a user runs `exasol version`
- **THEN** the launcher does not perform a separate automatic update check after the command
