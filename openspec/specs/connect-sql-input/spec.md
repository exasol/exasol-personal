# connect-sql-input Specification

## Purpose
TBD - created by archiving change add-connect-command-and-file-flags. Update Purpose after archive.
## Requirements
### Requirement: Inline SQL via --command

`exasol connect` SHALL accept a `-c`/`--command <SQL>` flag whose argument is one or more SQL statements. When the flag is supplied, the command SHALL execute the statements non-interactively and then exit, without starting the interactive shell or reading stdin.

The supplied SQL SHALL be split into statements on `;` terminators using the same splitting rules as the interactive shell (quote- and comment-aware), and each statement SHALL be executed in order against the database. Results SHALL be printed using the same formatter as the interactive shell, honoring `--json` and `--json` format options.

#### Scenario: Single statement

- **WHEN** the user runs `exasol connect -c "SELECT 1"`
- **THEN** the statement is executed and its result is printed
- **AND** the command exits without entering the interactive shell

#### Scenario: Multiple semicolon-separated statements

- **WHEN** the user runs `exasol connect -c "CREATE TABLE t(x INT); INSERT INTO t VALUES (1); SELECT * FROM t"`
- **THEN** each statement is executed in order
- **AND** the result of each result-producing statement is printed in order

#### Scenario: Trailing or empty statements are ignored

- **WHEN** the supplied SQL contains empty segments (e.g. a trailing `;` or `;;`)
- **THEN** empty segments are skipped and no error is raised for them

### Requirement: SQL script via --file

`exasol connect` SHALL accept a `-f`/`--file <path>` flag whose argument is the path to a SQL script file. When the flag is supplied, the command SHALL read the file, execute its `;`-separated statements non-interactively in order, and then exit, without starting the interactive shell or reading stdin.

The file contents SHALL be split and executed using the same rules as `--command`.

#### Scenario: Run a script file

- **WHEN** the user runs `exasol connect -f script.sql` and `script.sql` contains multiple `;`-separated statements
- **THEN** each statement is executed in order and results are printed

#### Scenario: Missing or unreadable file

- **WHEN** the file given to `--file` does not exist or cannot be read
- **THEN** the command exits with a non-zero status and a clear error message
- **AND** no statements are executed

### Requirement: Flag precedence and mutual exclusivity

The `--command` and `--file` flags SHALL be mutually exclusive. Supplying both SHALL cause the command to exit with a non-zero status and a clear error message before connecting to the database. When neither flag is supplied, `exasol connect` SHALL retain its existing interactive and stdin behavior unchanged.

#### Scenario: Both flags supplied

- **WHEN** the user runs `exasol connect -c "SELECT 1" -f script.sql`
- **THEN** the command exits with a non-zero status and an error stating the flags are mutually exclusive

#### Scenario: Neither flag supplied preserves existing behavior

- **WHEN** the user runs `exasol connect` with neither `--command` nor `--file`
- **THEN** the command behaves as before: interactive shell on a TTY, or reads SQL from stdin when piped

### Requirement: Non-interactive exit status

When run with `--command` or `--file`, `exasol connect` SHALL exit with a non-zero status if any executed statement fails, so that callers and scripts can detect errors.

#### Scenario: Statement fails

- **WHEN** a statement supplied via `--command` or `--file` fails to execute
- **THEN** the command reports the error
- **AND** exits with a non-zero status

