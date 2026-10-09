# cli-output-contract Specification

## Purpose
TBD - created by archiving change establish-cli-output-contract. Update Purpose after archive.
## Requirements
### Requirement: Successful command output SHALL be confined to stdout

The CLI SHALL write the primary result of a successful command to standard output, and SHALL write everything else a human reads — progress, context notices, prompts, and next-step guidance — to standard error, so that standard output carries only the requested result.

#### Scenario: Result goes to stdout, guidance does not

- **WHEN** a user runs a command that produces a result
- **THEN** the requested result is written to standard output
- **AND** explanatory text, prompts, and guidance are not written to standard output

#### Scenario: Piped output carries only the result

- **WHEN** a user pipes a command's standard output to another program
- **THEN** the receiving program reads only the primary result
- **AND** human-facing messages remain visible to the user on standard error

### Requirement: JSON output SHALL be the only content on stdout

When `--json` is selected, the CLI SHALL write only valid JSON to standard output for a successful command, and SHALL NOT write banners, prompts, notices, or call-to-action text to standard output.

#### Scenario: JSON stdout is parseable

- **WHEN** a user runs a command with `--json`
- **THEN** standard output contains only valid JSON
- **AND** no guidance, prompts, or notices appear on standard output

#### Scenario: Call-to-action guidance is absent under JSON

- **WHEN** a user runs a command with `--json` that would otherwise suggest a next step
- **THEN** the next-step guidance is not emitted on any stream

### Requirement: Next-step guidance SHALL be shown for text output and suppressed only for JSON

The CLI SHALL write call-to-action and next-step guidance to standard error. This guidance is textual help that any reader benefits from, including a non-interactive agent driving the CLI in a workflow, so the CLI SHALL NOT gate it on an interactive terminal. The CLI SHALL suppress it only when `--json` is selected, where consumers rely on structured output and branch on structured state fields instead of prose.

#### Scenario: Guidance is shown for text output

- **WHEN** a user or agent runs a command without `--json` and a relevant next step exists
- **THEN** the CLI shows the next-step guidance on standard error, whether or not standard error is an interactive terminal
- **AND** the primary result on standard output is unaffected

#### Scenario: Guidance is suppressed under JSON

- **WHEN** a command is run with `--json` and a relevant next step exists
- **THEN** the CLI does not emit the next-step guidance on any stream

### Requirement: Operational notices SHALL stay visible without corrupting output

The CLI SHALL write operational notices — information about what a command acted on, such as the resolved deployment directory or acceptance of the license — to standard error so they remain visible, including when standard output is piped or `--json` is selected, without affecting the parseability of standard output.

#### Scenario: Notice accompanies piped output

- **WHEN** a command emits an operational notice while its standard output is piped
- **THEN** the notice appears on standard error
- **AND** standard output remains the unmodified primary result

#### Scenario: Notice does not corrupt JSON

- **WHEN** a command emits an operational notice with `--json`
- **THEN** standard output remains valid JSON
- **AND** the notice appears on standard error

### Requirement: Expected failures SHALL be reported as errors

The CLI SHALL report expected failures — such as unsupported platform or backend combinations, invalid input, and validation failures — through the error path with a non-zero exit status, and SHALL NOT emit them as successful command output.

#### Scenario: Unsupported combination is an error

- **WHEN** a user runs a command with an unsupported platform or backend combination
- **THEN** the command fails with a non-zero exit status
- **AND** the failure is reported on standard error
- **AND** standard output does not contain a success payload

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

