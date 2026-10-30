## MODIFIED Requirements

### Requirement: Containers honor restart policy

Sidecars SHALL use their configured restart policy, defaulting to `Always`.
`OnFailure` SHALL restart a process after a nonzero exit; `Always` SHALL restart
after any process exit; `Never` SHALL leave an exited process stopped. Explicit
deployment stop and sidecar disable SHALL take precedence over automatic
restart.

#### Scenario: Process exit follows the selected policy

- **WHEN** a sidecar process exits with a controlled exit code
- **THEN** its subsequent running state matches the selected restart policy
