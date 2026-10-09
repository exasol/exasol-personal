## ADDED Requirements

### Requirement: Terminal messages SHALL follow command lifecycle phases
The CLI SHALL display all terminal messages queued during shared startup before command-specific execution and display remaining messages after successful command completion. In each phase, operational notices SHALL be written to stderr, primary results to stdout, and calls to action to stderr after that phase's other messages.

#### Scenario: Startup messages precede command execution
- **WHEN** shared startup queues terminal messages while resolving deployment context
- **THEN** all queued notices, results, and calls to action are displayed before command-specific execution begins
- **AND** each is displayed only once during the invocation

#### Scenario: Successful command displays its remaining messages
- **WHEN** a command completes successfully after the startup messages have been displayed
- **THEN** remaining operational notices and primary results are displayed after command completion on their respective streams
- **AND** remaining calls to action appear after those messages

#### Scenario: Command failure retains early context and error guidance
- **WHEN** a command fails after startup messages have been displayed
- **THEN** the already-displayed startup messages remain visible
- **AND** the command error is reported on stderr with a non-zero exit status
- **AND** any error-specific recovery guidance may follow the error on stderr

#### Scenario: JSON output remains machine-readable across phases
- **WHEN** a successful command produces JSON output and startup or completion notices exist
- **THEN** stdout contains only the JSON result
- **AND** notices appear on stderr
- **AND** calls to action are suppressed
