## RENAMED Requirements

- FROM: `### Requirement: Linux local shell commands fail explicitly`
- TO: `### Requirement: Linux local host access uses the caller's environment`

## MODIFIED Requirements

### Requirement: Linux local host access uses the caller's environment

For Linux local deployments, `exasol shell host` SHALL launch the shell named
by `SHELL`, falling back to `/bin/sh` when unset or empty. Explicit commands
following `--` SHALL execute with their original argument boundaries. Both
forms SHALL inherit the caller's environment, working directory, standard
input, standard output, and standard error. Execution failures SHALL produce
a nonzero CLI result. Host access SHALL work for initialized and stopped
deployments. Container shell commands SHALL report their unsupported status.

#### Scenario: Host shell requested on Linux

- **WHEN** a user runs `exasol shell host` for a Linux local deployment
- **THEN** the configured shell, or `/bin/sh` fallback, runs locally with the
  caller's environment, working directory, and standard streams

#### Scenario: Explicit host command requested on Linux

- **WHEN** a user runs `exasol shell host -- <command>` on Linux
- **THEN** the command receives the original arguments and standard input,
  its standard output and error reach the caller, and a process failure
  produces a nonzero CLI result

#### Scenario: Container shell requested on Linux

- **WHEN** a user runs `exasol shell container` for a Linux local deployment
- **THEN** the command fails with an explicit error identifying container shell
  access as unsupported on Linux
