## 1. Clear error for unresolved config set options

- [x] 1.1 Surface a clear, actionable error when `config set` cannot load the target
      deployment's configurable options (non-help invocation), naming the resolved deployment
      directory and the corrective action, instead of registering no flags.
- [x] 1.2 Preserve `config set --help` (and `help config set`): render help when options cannot
      be loaded, show options when they can.
- [x] 1.3 Preserve normal unknown-option errors for misspelled options against a resolvable
      deployment.

## 2. Verification

- [x] 2.1 Add coverage for the clear error when the deployment directory is not initialized or
      cannot be resolved.
- [x] 2.2 Add coverage that `config set --help` still renders without a resolvable deployment.
- [x] 2.3 Add regression coverage that a destroyed (i.e. initialized) local deployment still
      registers its infrastructure options for `config set`.
- [x] 2.4 Run repository validation used for the pull request (`task fmt`, `task lint`,
      `task build`, `task tests-unit`; `task tests-integration` deferred — needs cloud creds).
