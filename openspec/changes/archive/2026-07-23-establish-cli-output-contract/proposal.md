## Why

The CLI mixes primary output, human guidance, and call-to-action (CTA) prompts without a documented contract for which stream each uses or when it appears. Today the available-update hint is printed on stderr even under `--json`, `config set` and `config reset` write their primary result (the effective configuration) to stderr, `cache unlock` writes its confirmation to stdout, and several next-step hints are fused into notices. Scripts and agents cannot rely on a predictable output contract, and the best-practice that governs this behavior does not yet define what a CTA is or when it should be shown.

## What Changes

- Establish a documented output contract: successful primary output on stdout only; human notices, prompts, and guidance on stderr; expected failures on the error path.
- Define three user-observable output kinds — primary output, operational notice, and call-to-action — and the visibility rules for each.
- Suppress call-to-action / next-step guidance only when `--json` is selected; keep it on stderr for text output (interactive or not) so agents and scripts driving the CLI still receive it.
- Suppress the available-update hint under `--json` (today it is emitted even under `--json`).
- Route `config set` and `config reset` primary results to stdout, and reclassify their "run `exasol deploy`" line as suppressible guidance. Same-preset `init` patch output follows the same rule. A machine-readable `--json` form for these commands is deferred.
- Route the `cache unlock` confirmation to stderr as an operational notice.
- Revise the "Keep CLI output in the command layer" best-practice to document the contract.

## Capabilities

### New Capabilities

- `cli-output-contract`: the user-observable contract for how CLI commands use stdout and stderr, when call-to-action guidance appears, and how expected failures are reported.

### Modified Capabilities

- `launcher-version-check`: the available-update hint becomes suppressible interactive guidance.
- `deployment-reconfiguration`: `config set` / `config reset` (and same-preset `init` patch) print the effective configuration to stdout; the apply guidance becomes suppressible call-to-action output.
- `runtime-artifact-cache`: cache unlock reports its result as an operational notice on stderr.

## Impact

- User-facing CLI output routing across most commands.
- Automation reading `--json` gets clean stdout and no CTA prose on any stream.
- Text-output consumers, including non-interactive agents and scripts, still receive next-step guidance on stderr.
- Scripts that captured `config set` / `config reset` values from stderr must read stdout instead.
- Deferred follow-ups (separate changes): `--json` for `config set` / `config reset`, `deploy`, `install`, `cache clean`, `presets export`; folding `connect` into the shared output contract; error-path flush semantics for queued output.
