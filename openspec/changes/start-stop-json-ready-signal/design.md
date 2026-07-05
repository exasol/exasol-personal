## Context

`exasol start` and `exasol stop` currently accept only human-oriented lifecycle flags. They run deployment operations, write logs through the normal logging path, and `start` queues connection instructions as final terminal output after the database is ready. Automation users need a completion signal on stdout that remains parseable when logs are emitted to stderr.

The repository already has command-level JSON patterns for `status`, `info`, `cache`, `version`, and `connect`. Lifecycle commands should follow the same boundary: command code owns user-visible rendering, while deployment code owns state changes and backend orchestration.

## Goals / Non-Goals

**Goals:**

- Add `--json` to `exasol start` and `exasol stop`.
- Emit one final JSON document to stdout after successful completion.
- Keep deployment logs, notices, and connection instructions out of stdout in JSON mode.
- Preserve existing non-JSON lifecycle behavior.
- Cover the contract with focused tests for flag registration, rendering, and stdout cleanliness.

**Non-Goals:**

- No reduction of start or stop duration.
- No change to `exasol status --json`.
- No NDJSON or streaming progress protocol.
- No backend-specific JSON schema beyond lifecycle completion state.

## Decisions

**Render lifecycle JSON in `cmd/exasol`.** The command layer will register `--json`, call the existing deployment operation, and render a small completion document after success. This matches existing command output ownership and avoids making `internal/deploy` depend on CLI presentation concerns. Alternative: have `deploy.Start` and `deploy.Stop` return serialized JSON. Rejected because deployment orchestration should remain independent from terminal output formats.

**Use a compact completion schema.** The JSON document will include `deploymentState` and `databaseReady`. For `start`, the state is `running` and `databaseReady` is `true`. For `stop`, the state is `stopped` and `databaseReady` is `false`. Alternative: reuse the full `info` or `status` report. Rejected because the story asks for a final ready signal, and a minimal schema is easier for scripts to consume.

**Suppress connection instructions only in lifecycle JSON mode.** `start` currently prints refreshed connection instructions after success. In JSON mode, those instructions would corrupt stdout, so the command will skip `addConnectionInstructionsTerminalOutput` and emit only the completion document. Non-JSON behavior remains unchanged.

**Leave error handling unchanged.** If start or stop fails, the command exits non-zero through the existing error path. The JSON contract applies to successful completion; structured lifecycle failure JSON is out of scope for this story. Alternative: wrap errors in JSON when `--json` is set. Rejected because the acceptance criteria only require exit status to reflect failure, not a new error schema.

## Risks / Trade-offs

- **Global `commonFlags.OutputJson` is shared across commands** -> Register the existing output flag on lifecycle commands and add focused tests so future commands do not accidentally omit it.
- **Terminal notices can be added by root pre-run** -> Because notices print to stderr and primary output prints to stdout, stdout remains machine-readable; tests should exercise command rendering directly.
- **The schema is intentionally small** -> Future consumers may ask for more fields, but the minimal document satisfies readiness automation without committing to full deployment metadata.
