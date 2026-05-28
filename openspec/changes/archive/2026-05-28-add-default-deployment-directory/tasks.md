## 1. Deployment Directory Resolution

- [x] 1.1 Add a command-layer resolver for explicit flag, recognized current deployment directory, and default deployment directory precedence.
- [x] 1.2 Update deployment directory flag handling so omitted and explicit values can be distinguished.
- [x] 1.3 Wire resolution into root pre-run before compatibility checks, deployment file logging, version update hints, and command execution.
- [x] 1.4 Emit a human-facing default-directory message on stderr/logging without corrupting JSON stdout.

## 2. Command Behavior

- [x] 2.1 Ensure `init` and `install` create and use the default deployment directory when resolution selects it.
- [x] 2.2 Keep initialized-state enforcement for commands that require initialized deployments and include the resolved path in errors.
- [x] 2.3 Change `status` so uninitialized resolved directories report `not_initialized` instead of failing solely because initialization is missing.
- [x] 2.4 Add active deployment directory to status text and JSON output.

## 3. Documentation and Help

- [x] 3.1 Update root and command help text to explain the default directory and `--deployment-dir` override.
- [x] 3.2 Update end-user documentation to describe precedence rules and the default workflow.
- [x] 3.3 Update cloud cleanup guidance to explain why deployment directories should be kept until resources are destroyed.

## 4. Tests and Verification

- [x] 4.1 Add unit tests for explicit `--deployment-dir` precedence, recognized current directory precedence, and default fallback.
- [x] 4.2 Add tests for default-directory creation during `init` or `install`.
- [x] 4.3 Add tests for initialized-state errors including the resolved path.
- [x] 4.4 Add tests for `status` output including active deployment directory in text and JSON.
- [x] 4.5 Add tests for `status` reporting `not_initialized` on uninitialized explicit and default directories.
- [x] 4.6 Run formatting, unit tests, and focused integration tests for deployment-directory and status behavior.
