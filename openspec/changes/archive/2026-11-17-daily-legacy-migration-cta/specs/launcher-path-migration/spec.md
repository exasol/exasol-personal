## MODIFIED Requirements

### Requirement: Migration SHALL keep a legacy deployment in use at its legacy location
Startup path migration SHALL keep a legacy deployment at its legacy location while a local deployment there runs, while its running state cannot be determined, or while another launcher process holds its lock. The CLI SHALL present deferred-migration guidance according to the daily CTA requirement. Migration SHALL move the other legacy items.

#### Scenario: Running local deployment is kept
- **WHEN** a local deployment at the legacy location is running
- **THEN** that deployment remains usable at its legacy location
- **AND** migration moves the other eligible legacy items
- **AND** the CLI presents guidance according to the daily CTA requirement

#### Scenario: Deployment with unknown running state is kept
- **WHEN** the CLI cannot determine whether a local deployment at the legacy location is running
- **THEN** the deployment stays at its legacy location
- **AND** the CLI presents guidance according to the daily CTA requirement
- **AND** the probe failure remains visible as an operational diagnostic naming the cause

#### Scenario: Locked legacy deployment is kept
- **WHEN** another launcher process holds the lock of a deployment at the legacy location
- **THEN** the deployment stays at its legacy location
- **AND** the CLI presents guidance according to the daily CTA requirement

### Requirement: The requested command SHALL run after a migration warning
When startup migration fails to move or delete a legacy item, the CLI SHALL emit a warning on standard error naming the item and the cause, continue with the remaining items, and then dispatch the requested command.

#### Scenario: Failed migration step warns and the command runs
- **WHEN** startup migration fails to move or delete a legacy item, for example because of an I/O error
- **THEN** the CLI emits a warning on standard error naming the item and the cause
- **AND** the CLI migrates the remaining legacy items
- **AND** the CLI runs the requested command

## ADDED Requirements

### Requirement: Deferred migration guidance SHALL be a daily call to action
For each retained legacy deployment, the CLI SHALL present migration guidance as a call to action on standard error at most once per host-local calendar date when reminder state can be persisted. Text invocations, including noninteractive invocations, SHALL be eligible. Guidance SHALL name the old and target paths, explain the layout change and reason for deferral, state the applicable recovery action and explain that legacy support is temporary. Actual migration failures and naming collisions SHALL follow the operational diagnostic contract independently of reminder frequency.

#### Scenario: First text invocation presents running-deployment guidance
- **WHEN** a running legacy deployment has its first eligible text invocation on a local calendar date
- **THEN** standard error presents one migration CTA naming both paths and explaining that stopping the deployment lets a later invocation move it automatically
- **AND** the CTA identifies the applicable stop command and explains that legacy support is temporary
- **AND** the requested command proceeds

#### Scenario: Repeated text invocations use the saved date
- **WHEN** another process invokes the launcher on the same local date after a reminder was claimed for that deployment
- **THEN** the requested command proceeds with its ordinary output and the saved reminder date remains current
- **AND** the total migration CTA count for that deployment and date remains one

#### Scenario: A new local date permits another reminder
- **WHEN** the local calendar date changes and migration is still deferred
- **THEN** the next eligible text invocation presents one CTA for the new date

#### Scenario: Separate deployments have separate reminder dates
- **WHEN** two retained deployments are encountered on the same date and only one has a current reminder claim
- **THEN** the other deployment receives its eligible CTA independently

#### Scenario: Clock rollback preserves suppression
- **WHEN** the host clock moves to an earlier local date than a saved reminder claim
- **THEN** that claim continues to suppress the deployment's CTA until the local date advances beyond it

#### Scenario: Noninteractive text output includes guidance
- **WHEN** an eligible text invocation runs with redirected stdin or stdout
- **THEN** the migration CTA is written to standard error according to the same daily policy
- **AND** standard output contains the requested command's primary output

#### Scenario: JSON preserves the next text reminder
- **WHEN** a JSON invocation encounters a retained deployment before its daily reminder is claimed
- **THEN** standard output contains the requested command's JSON result
- **AND** standard error contains only applicable operational notices and diagnostics
- **AND** a later text invocation on the same date remains eligible for the migration CTA

#### Scenario: Locked deployment guidance explains safe recovery
- **WHEN** an eligible invocation encounters a locked legacy deployment
- **THEN** its CTA asks the user to close launcher operations using the deployment
- **AND** unlock guidance is conditional on confirming that no launcher operation is running

#### Scenario: Unknown running state guidance explains the deferral
- **WHEN** an eligible invocation cannot determine the legacy deployment's running state
- **THEN** its CTA explains that migration is deferred to protect a potentially running deployment and identifies the state-check problem and applicable diagnostic action

#### Scenario: Collision diagnostics remain visible
- **WHEN** a legacy and current deployment collide by name after a daily reminder has already been claimed
- **THEN** the invocation still reports the collision and both paths through the operational diagnostic path

#### Scenario: Concurrent invocations claim one reminder
- **WHEN** multiple eligible processes encounter the same retained deployment on the same local date with writable reminder storage
- **THEN** one process claims that deployment's daily CTA
- **AND** every process proceeds to its requested command

#### Scenario: Invalid or unavailable reminder storage preserves command execution
- **WHEN** reminder storage is missing, invalid or cannot be written
- **THEN** the CLI permits best-effort migration guidance and proceeds to the requested command
- **AND** reminder persistence errors are available through debug logging

#### Scenario: Migration succeeds after today's reminder
- **WHEN** a deployment becomes eligible to move after its daily CTA was claimed
- **THEN** the next invocation moves it according to the existing migration contract
- **AND** migration-specific reminder artifacts are retired
- **AND** the deployment's data remains available at its new location
