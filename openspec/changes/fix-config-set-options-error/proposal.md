## Why

`exasol config set` exposes preset-specific infrastructure options (`--cpu-count`,
`--memory-mb`, `--data-size-gb`, `--ports`) dynamically: it resolves them from the target
deployment directory before Cobra parses arguments. When that resolution fails — because
`--deployment-dir` was omitted (and the current directory is not a deployment) or the target
directory is not an initialized deployment — the options are silently not registered, and the
command then fails during flag parsing with a misleading `unknown flag: --<option>`. Users
conclude the option was removed and that `config set` is broken (SPOT-31462).

## What Changes

- When `config set` cannot load the target deployment's configurable options, fail with a
  clear, actionable error that names the resolved deployment directory and tells the user to
  initialize it (`exasol init` / `exasol install`) or point `--deployment-dir` at an existing
  deployment — instead of reporting supplied options as unknown flags.
- Preserve `config set --help`: it still renders (base help when options cannot be loaded, the
  full option list when they can).
- Preserve normal unknown-option errors for genuinely misspelled options against a resolvable
  deployment.
- No change to state gates: `config set` in an initialized deployment (including after
  `destroy`, which returns the deployment to the initialized state) continues to work; running
  and stopped deployments remain refused by design.

## Capabilities

### Modified Capabilities

- `deployment-reconfiguration`: clarifies the error behaviour of `config set` when configurable
  options cannot be loaded for the target deployment directory.

## Impact

- User-facing error message for `exasol config set` when the deployment directory cannot be
  resolved or is not initialized.
- No change to configuration semantics, deployment state gates, or the other config
  subcommands (`get`, `reset`).
