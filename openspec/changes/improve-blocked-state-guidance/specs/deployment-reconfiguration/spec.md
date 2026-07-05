# deployment-reconfiguration Specification

## ADDED Requirements

### Requirement: State-guarded commands SHALL surface recovery guidance when blocked by deployment state

State-guarded lifecycle commands (`install`/`deploy`, `connect`, `start`, `stop`) SHALL surface state-appropriate recovery guidance when the current deployment state does not permit the command, instead of a generic error. The message SHALL name the resolved deployment directory, report the current deployment state, and give the recovery command(s) for that state. The command SHALL NOT present a known, recoverable state as an "unexpected" error, and SHALL NOT require the user to run a separate command to learn how to recover. The recovery guidance SHALL be the same guidance reported by `exasol status` for that state.

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

#### Scenario: Connect blocked before the database is running

- **WHEN** a user runs `exasol connect` while the deployment is not running (for example
  initialized, stopped, or interrupted)
- **THEN** the command fails without connecting
- **AND** the message reports the current state, names the deployment directory, and gives
  the recovery command to reach a running database
- **AND** the message does not label the state as "unexpected"

#### Scenario: Start or stop blocked by an incompatible state

- **WHEN** a user runs `exasol start` or `exasol stop` while the deployment state does not
  permit that operation (for example starting a not-yet-deployed deployment, or stopping a
  stopped one)
- **THEN** the command fails without changing the deployment
- **AND** the message reports the current state, names the deployment directory, and gives
  the state-appropriate recovery command
- **AND** the message does not label the state as "unexpected"

#### Scenario: Permitted states still run

- **WHEN** a user runs a lifecycle command in a state that permits it (for example
  `deploy` when initialized, deployment failed, running, deploy in progress, or interrupted
  during deploy; `connect` when running)
- **THEN** the command proceeds as before
