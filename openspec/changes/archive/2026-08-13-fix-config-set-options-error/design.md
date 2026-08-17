## Context

`config set` accepts preset-specific infrastructure options as flags. Because the available
options depend on the deployment's persisted preset, the launcher registers those flags
dynamically at startup — before Cobra parses arguments — by resolving the options from the
target deployment directory. The other config subcommands (`get`, `reset`) take positional
option names rather than option flags, so they are unaffected.

The dynamic registration currently swallows any resolution error: if the deployment directory
cannot be resolved or is not initialized, no option flags are registered and the failure is
invisible. Cobra then parses the user's `--<option>` and reports `unknown flag`, which runs
before the command's "deployment must be initialized" pre-run gate can produce a helpful
message. The result is a misleading error (SPOT-31462).

## Goals / Non-Goals

**Goals:**
- Replace the misleading `unknown flag: --<option>` with a clear, actionable error that names
  the resolved deployment directory and the corrective action.
- Keep `config set --help` working when options cannot be loaded.
- Keep genuine unknown-option errors intact for typos against a resolvable deployment.

**Non-Goals:**
- No change to which deployment states permit configuration (running/stopped stay refused by
  design; initialized/destroyed keep working).
- No change to configuration semantics or to `config get` / `config reset`.
- No change that would make `config set` in the stopped state succeed (a separate design
  question).

## Decisions

**Surface the resolution error at pre-registration instead of swallowing it.**
When the invocation is `config set` and it is not a help request, a failure to resolve the
deployment's configurable options is reported as a clear error naming the resolved deployment
directory, rather than being discarded. This fires before Cobra's flag parsing, so the user
never sees the `unknown flag` message for a dir that simply could not be loaded.

**Keep help resilient.**
For `config set --help` (or `help config set`), resolution failures remain non-fatal so the
base help still renders; when options resolve, they are shown as before.

**Do not mask real unknown options.**
When the deployment resolves successfully, its options are registered and Cobra's normal
unknown-flag handling continues to reject misspelled or unsupported options.

## Risks / Trade-offs

- The clear error fires earlier (at flag registration) than the existing initialized-deployment
  pre-run gate; wording is kept consistent with that gate so users get one coherent message.
- Message text may evolve; tests assert on stable, meaningful substrings rather than exact
  strings where practical.
