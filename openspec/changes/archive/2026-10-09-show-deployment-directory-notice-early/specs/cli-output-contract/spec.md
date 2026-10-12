## ADDED Requirements

### Requirement: Terminal messages SHALL follow command lifecycle phases
The CLI SHALL display all terminal messages queued during shared startup before command-specific execution and display remaining committed notices and primary results after command completion, whether the command succeeds or fails. In each phase, operational notices SHALL be written to stderr and primary results to stdout. Calls to action SHALL follow the phase's other messages on success; on failure, recovery guidance SHALL follow the command error on stderr. A command SHALL commit a primary result only after all expected failures that could invalidate that result have been ruled out. JSON output SHALL suppress prose calls to action, including for structured failures.

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
- **AND** notices and primary results committed during the command are displayed on their respective streams before the command error
- **AND** the command error is reported on stderr with a non-zero exit status
- **AND** any committed error-specific recovery guidance follows the error on stderr

#### Scenario: Expected failure does not publish a success payload
- **WHEN** a command encounters an expected failure before its primary result is ready
- **THEN** no success payload is committed to stdout
- **AND** any already-committed operational notices remain visible on stderr

#### Scenario: JSON output remains machine-readable across phases
- **WHEN** a successful command produces JSON output and startup or completion notices exist
- **THEN** stdout contains only the JSON result
- **AND** notices appear on stderr
- **AND** calls to action are suppressed

#### Scenario: Structured JSON failure suppresses prose guidance
- **WHEN** a command returns a structured JSON failure result
- **THEN** stdout contains only that JSON result
- **AND** notices appear on stderr
- **AND** prose calls to action are suppressed
