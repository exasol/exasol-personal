Each implementation task below maps to exactly one code-only commit that passes
the full check suite. Group 5 is validation and produces no commit.

## 1. Probe definition

- [x] 1.1 Add `readinessProbe` to the container subset, with `exec`,
  `httpGet`, and `tcpSocket` handlers, a port reference by number or name, the
  timing fields, their defaults, and validation naming the offending field,
  leave the probe out of the container fingerprint, and declare the MCP catalog
  readiness probe. Verify `task tests-unit` covers parsing both port reference
  forms, the defaults, each validation error, and an unchanged fingerprint
  after a probe edit. Verify `task tests-launcher` covers enabling the MCP
  sidecar saving its probe with defaults and an invalid saved probe failing
  with the field name.

## 2. Health in status

- [x] 2.1 Add command execution and the container start time to the container
  runtime, evaluate the readiness probe of a running sidecar during status, and
  report `health` and `healthError` in text and JSON. Verify `task tests-unit`
  covers each probe handler, the initial delay, the startup window, a sidecar
  without a probe, and the text lines. Add live tests for serving, starting,
  and failing health and for an edited probe applying without recreating the
  container.

## 3. Enable waits for readiness

- [x] 3.1 Make enable wait while health is starting, bounded by the probe's
  startup window, add `--no-wait`, add the calls to action for starting and
  failing health, and regenerate the CLI reference. Verify `task tests-unit`
  covers a wait that ends serving, one that ends failing, the wall-clock bound,
  cancellation, `--no-wait`, and the calls to action. Add live tests for enable
  returning serving, failing, and starting with `--no-wait`, and assert the MCP
  sidecar serves after enable.

## 4. Documentation

- [x] 4.1 Document the readiness probe, the health values, the enable wait,
  and `--no-wait` in the user sidecar page, and update the unreleased sidecar
  changelog entry. Verify `task docs-check` passes.

## 5. Validation

- [x] 5.1 Run `task all` and confirm formatting, linting, unit tests and the
  launcher tests pass on the finished branch.
