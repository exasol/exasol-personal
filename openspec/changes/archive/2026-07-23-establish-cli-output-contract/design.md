## Context

Terminal output is queued through command-layer helpers and flushed once at the end of a run: one helper writes queued messages to stdout, another writes them to stderr. There is no notion of a suppressible call-to-action, and no output is gated on whether a stream is an interactive terminal. `--json` suppression is applied ad hoc, per command. An audit of every command found the misroutings and fused CTAs enumerated in `tasks.md` (notably the always-on update hint, `config set`/`reset` results on stderr, and the `cache unlock` confirmation on stdout).

## Goals / Non-Goals

**Goals:**
- A single, documented contract for stdout/stderr usage and CTA visibility.
- A reusable mechanism for suppressible call-to-action output.
- Correct routing for the commands whose output is currently misrouted.

**Non-Goals:**
- Adding `--json` to commands that do not support it today (`config set`/`reset`, `deploy`, `install`, `cache clean`, `presets export`).
- Re-plumbing `connect`, which writes directly to its own streams and has its own `--json`/`--csv` handling.
- Changing when queued output is flushed relative to command errors.

## Decisions

**Three output kinds.** Primary output (stdout; JSON under `--json`), operational notice (stderr, always shown — context needed to interpret the result, e.g. the resolved deployment directory or license acceptance), and call-to-action (stderr, decorative next-step guidance). The discriminator is the delete-test: removing a CTA changes neither the result nor its correctness reporting.

**CTA visibility.** Call-to-action output is suppressed only when `--json` is selected. It is NOT gated on an interactive terminal: this tool is agent-facing, and a non-interactive agent running a command in text mode benefits from next-step guidance just as a human does. Stdout purity is already guaranteed by stream separation (CTAs go to stderr), so a TTY gate would add no protection to stdout while blinding the exact intelligent consumers who benefit. Under `--json`, consumers want structured output and branch on structured state fields (e.g. `info`'s `deploymentState`), so prose CTAs are suppressed there. Ephemeral rendered output (progress, spinners, color) is the category that would warrant a TTY gate, and this tool queues none of it today.

**New helper.** Introduce a command-layer call-to-action helper (alongside the existing stdout and stderr-notice helpers). CTAs currently fused into notices move to this helper; operational notices stay on the notice helper. Whether queued CTAs are rendered is decided once at flush time from the `--json` flag, which keeps the routing decision in one place and lint-enforceable.

**Config primary output.** `config set`, `config reset`, and the same-preset `init` patch print the effective configuration through the stdout path; their "run `exasol deploy`" apply guidance becomes call-to-action output.

## Risks / Trade-offs

- CTAs now appear on stderr for non-interactive text runs (logs, CI, piped stderr) where a strict human-CLI convention would hide them. This is a deliberate trade for an agent-facing tool: stdout stays pure regardless, and the guidance helps any reader. Consumers that want no prose use `--json`.
- `config set`/`reset` gain stdout output where they previously printed only to stderr; scripts that captured stderr for the values must read stdout. A `--json` form is a deferred follow-up.
