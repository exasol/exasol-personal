## ADDED Requirements

### Requirement: Status reports sidecar health from its readiness probe

Status SHALL report `health` for a running sidecar that declares a readiness
probe, and an empty `health` otherwise. The launcher SHALL run an `exec` probe
inside the container and SHALL run an `httpGet` or `tcpSocket` probe against
the probed port's published loopback address. An `exec` probe SHALL succeed on
exit status 0, an `httpGet` probe on a response status from 200 through 399,
and a `tcpSocket` probe on an accepted connection, each within
`timeoutSeconds`. `health` SHALL be `starting` until `initialDelaySeconds` has
elapsed since the container started. Afterwards it SHALL be `serving` when the
probe succeeds. A failed probe SHALL make it `starting` until
`initialDelaySeconds + periodSeconds × failureThreshold` has elapsed since the
container started, and `failing` afterwards. Status SHALL report the cause of
the latest probe failure as `healthError`. Text and JSON output SHALL carry
`health` and `healthError`, separately from `state` and `reconciliation`. A
failing probe SHALL leave the container running under its restart policy.

#### Scenario: A sidecar whose readiness probe succeeds is serving

- **WHEN** a running sidecar's readiness probe succeeds
- **THEN** text and JSON status report health as serving and no health error

#### Scenario: A sidecar inside its probe window is starting

- **WHEN** a running sidecar's readiness probe fails before its startup window
  has elapsed
- **THEN** status reports health as starting with the probe failure as the
  health error

#### Scenario: A sidecar past its probe window is failing

- **WHEN** a running sidecar's readiness probe fails after its startup window
  has elapsed
- **THEN** status reports health as failing with the probe failure as the
  health error, while state reports running

#### Scenario: A sidecar without a readiness probe reports no health

- **WHEN** a running sidecar declares no readiness probe
- **THEN** status reports an empty health and no health error

### Requirement: Enable waits for a starting sidecar

When enable leaves a sidecar with health `starting`, it SHALL re-evaluate
status at least once per second until health is no longer `starting` or the
startup window of its readiness probe has elapsed since the first evaluation,
and SHALL report the last result. `exasol sidecar enable <name> --no-wait`
SHALL report the first result without waiting. When the reported health is
`starting` or `failing`, text output SHALL include a call to action to run
`exasol sidecar status <name>`. Enable SHALL succeed whatever the reported
health.

#### Scenario: Enable returns once the sidecar serves

- **WHEN** the user enables a sidecar whose readiness probe succeeds after
  several seconds
- **THEN** enable reports health as serving

#### Scenario: Enable reports a sidecar that never serves

- **WHEN** the user enables a sidecar whose readiness probe keeps failing
- **THEN** enable returns after the startup window with health failing, the
  probe failure, and a call to action to run `exasol sidecar status <name>`

#### Scenario: Enable without waiting reports a starting sidecar

- **WHEN** the user enables a sidecar with `--no-wait` before its readiness
  probe succeeds
- **THEN** enable returns at once with health starting and a call to action to
  run `exasol sidecar status <name>`
