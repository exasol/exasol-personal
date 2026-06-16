## Context

`exasol info` is used by humans and automation to understand a deployment directory. The final experience needs to cover both first-run discovery, where no deployment exists yet, and initialized deployment discovery, where scripts need structured deployment and connection information.

## Goals / Non-Goals

**Goals:**
- Provide a helpful human-readable `exasol info` experience across deployment states.
- Provide valid JSON from `exasol info --json` for automation without terminal-only text on stdout.
- Make not-yet-initialized deployment directories a reportable state instead of an error case.
- Keep state-specific next steps focused on what the user can do next.
- Keep the JSON contract stable enough for scripts and agents to branch on deployment state and read connection-relevant fields.

**Non-Goals:**
- No changes to deployment creation, provisioning, start, stop, destroy, or remove behavior.
- No changes to the meaning of existing deployment states.
- No replacement of diagnostic commands that expose lower-level troubleshooting information.
- No terminal guidance prose in JSON output.

## Decisions

**Separate human guidance from machine-readable state.**
Text output should include next-step guidance because it is read by users in a terminal. JSON output should stay concise and structured so scripts can parse it without filtering explanatory text.

**Treat missing deployment state as an explicit state.**
When the resolved deployment directory is not initialized, `info` should report `not_initialized` and the resolved directory instead of failing solely because setup has not happened yet.

**Show next steps according to the current state.**
Text output should guide the user toward `presets list` and `install` before initialization, `deploy` after initialization, `connect` or `stop` when running, and `start` or `destroy` when stopped.

**Expose connection details only when they are meaningful.**
JSON output should include connection-relevant details for running deployments and omit them when the deployment state does not have stable connection information.

## Risks / Trade-offs

- Scripts that expected `exasol info` to fail before initialization may need to branch on `deploymentState` instead; this is intentional because `not_initialized` is now a valid reported state.
- Text output wording may evolve, so automation should consume `--json` rather than parse human-readable guidance.
