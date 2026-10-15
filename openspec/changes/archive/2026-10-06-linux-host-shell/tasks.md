## 1. Execution environments

- [x] 1.1 Accept optional child-process environment overrides on the command
  contract shared by direct, runner, and SSH execution. Preserve the command's
  own input and argument boundaries, and keep values out of process arguments.
  Verify delivery, preserved input, and rejected variable names with
  `task tests-unit`.

## 2. Host access

- [x] 2.1 Support Linux local host shells and explicit host commands,
  preserving argument boundaries, environment, working directory, streams, and
  failures. Document shell selection and Linux usage. Verify runtime and
  launcher tests with `task tests-unit` and `task tests-launcher`, regenerate
  the CLI reference, and run formatting, linting, and documentation checks.

## Commit and completion boundaries

Each numbered task is one independently verifiable commit. Keep updates to
existing behavior documentation with their implementation. Run the full
relevant checks for each commit. Keep all files in this active change
uncommitted and update checkboxes locally as work completes.

After all implementation tasks pass, sync the delta spec, archive this change,
and make the pure OpenSpec result the final branch commit. Preserve the user's
one-line Conventional Commit format.

## Scenario-to-test ownership

Linux shell scenarios below are MODIFIED. Each has exactly one owning test,
identified for review with the `openspec` marker. Parameterization remains an
instance of that test.

### exasol-local-deployment

- Host shell requested on Linux: `test_linux_host_shell` (launcher,
  parameterized by configured shell and fallback).
- Explicit host command requested on Linux: `test_linux_host_command`
  (launcher, parameterized by process success and failure).
- Container shell requested on Linux:
  `TestLocalBackend_LinuxContainerShellIsUnsupported` (backend unit).

### Fixture and execution details

Launcher tests run without provisioned infrastructure and skip on other
platforms. Supplementary unit tests cover shell selection, stream wiring, and
environment delivery through each execution transport.
