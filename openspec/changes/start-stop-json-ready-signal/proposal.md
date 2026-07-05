## Why

Automation users need `exasol start` and `exasol stop` to provide a stable readiness signal without scraping human log output or polling `exasol status --json` in a loop. This matters now because app builders and CI pipelines use lifecycle commands in scripted workflows where stdout must remain machine-readable.

## What Changes

- Add `--json` support to `exasol start` and `exasol stop`.
- Emit exactly one final JSON document on stdout after a successful lifecycle command completes.
- Include deployment state and database readiness fields so callers can wait for running or stopped outcomes without parsing logs.
- Keep default non-JSON lifecycle output unchanged.
- Keep logs, notices, and connection instructions out of stdout when lifecycle JSON mode is selected.

## Capabilities

### New Capabilities

- `lifecycle-json-output`: machine-readable completion output for deployment lifecycle commands.

### Modified Capabilities

<!-- None. -->

## Impact

- `cmd/exasol`: register lifecycle JSON flags and route JSON output to stdout.
- `internal/deploy`: expose or reuse lifecycle completion state data for rendering.
- Tests: add focused command/unit coverage and integration coverage for stdout JSON cleanliness.
