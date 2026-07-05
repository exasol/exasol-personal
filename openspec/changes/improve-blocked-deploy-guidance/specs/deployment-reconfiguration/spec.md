# deployment-reconfiguration Specification

## ADDED Requirements

### Requirement: Install and deploy SHALL surface recovery guidance when blocked by deployment state

`exasol install` and `exasol deploy` SHALL surface state-appropriate recovery guidance when the current deployment state does not permit deployment, instead of a generic error. The message SHALL name the resolved deployment directory, report the current deployment state, and give the recovery command(s) for that state. The command SHALL NOT present a known, recoverable state as an "unexpected" error, and SHALL NOT require the user to run a separate command to learn how to recover. The recovery guidance SHALL be the same guidance reported by `exasol status` for that state.

#### Scenario: Deploy blocked by an interrupted non-deploy operation

- **WHEN** a user runs `exasol install` or `exasol deploy` while the deployment is
  interrupted during an operation other than deploy (for example destroy)
- **THEN** the command fails without deploying
- **AND** the message reports the current deployment state
- **AND** the message names the resolved deployment directory
- **AND** the message tells the user which operation was interrupted and the recovery
  command to run (for example, run `destroy`)
- **AND** the message does not label the state as "unexpected"

#### Scenario: Deploy blocked by a stopped deployment

- **WHEN** a user runs `exasol install` or `exasol deploy` for a stopped deployment
- **THEN** the command fails without deploying
- **AND** the message reports the stopped state and points the user toward `start` or
  `destroy`

#### Scenario: Deploy blocked by a foreign operation in progress

- **WHEN** a user runs `exasol install` or `exasol deploy` while a non-deploy operation is
  recorded as in progress
- **THEN** the command fails without deploying
- **AND** the message reports the state and the state-appropriate recovery guidance

#### Scenario: Permitted states still deploy

- **WHEN** a user runs `exasol install` or `exasol deploy` in a state that permits
  deployment (initialized, deployment failed, running, a deploy operation in progress, or
  interrupted during deploy)
- **THEN** deployment proceeds as before
