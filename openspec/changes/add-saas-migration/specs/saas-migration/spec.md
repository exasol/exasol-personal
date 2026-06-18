# saas-migration Specification

## Purpose

Define how the launcher authenticates to an Exasol SaaS account, prepares connectivity, and migrates a local deployment's schema, table data, and database objects into a SaaS database.

## ADDED Requirements

### Requirement: SaaS access SHALL require a defined account token
The launcher SHALL require a stored SaaS account token before performing any SaaS account or migration operation other than defining the token.

#### Scenario: Token-gated command without a token
- **WHEN** a user invokes a SaaS allow-ip, test-connection, or migration command
- **AND** no SaaS token is defined for the deployment
- **THEN** the launcher refuses the command with a non-zero exit
- **AND** the launcher instructs the user to define a token first

#### Scenario: Token-gated command with a token
- **WHEN** a user invokes a token-gated SaaS command
- **AND** a SaaS token is defined for the deployment
- **THEN** the launcher proceeds with the command

### Requirement: The launcher SHALL define and validate a SaaS account token
The launcher SHALL let a user define a SaaS account token, validate it against the SaaS API, and persist it only when validation succeeds.

#### Scenario: Valid token is stored
- **WHEN** a user provides a SaaS account token
- **AND** the SaaS API accepts the token
- **THEN** the launcher stores the token in deployment secrets
- **AND** the token is masked in command output

#### Scenario: Invalid token is rejected
- **WHEN** a user provides a SaaS account token
- **AND** the SaaS API rejects the token
- **THEN** the launcher does not store the token
- **AND** the launcher reports that the token is invalid

#### Scenario: User inspects or clears the token
- **WHEN** a user requests to show or clear the stored SaaS token
- **THEN** the launcher reports the masked token or removes it accordingly

### Requirement: The launcher SHALL support interactive SaaS login
The launcher SHALL provide an interactive login flow that obtains a SaaS account token and stores it like a directly defined token.

#### Scenario: Interactive login obtains a token
- **WHEN** a user starts interactive SaaS login
- **AND** the login flow completes successfully
- **THEN** the launcher stores the obtained token in deployment secrets

#### Scenario: Interactive login is unavailable
- **WHEN** a user starts interactive SaaS login
- **AND** the interactive flow is not yet available
- **THEN** the launcher directs the user to define a token directly instead

### Requirement: The launcher SHALL manage the SaaS allowed-IP list
The launcher SHALL add an IP address or range to a SaaS database's allowed-IP list so the local source database's outbound connection is accepted.

#### Scenario: Auto-detected egress IP is added
- **WHEN** a user requests allow-ip without specifying an address
- **THEN** the launcher detects the deployment's public egress IP
- **AND** the launcher adds that IP to the SaaS allowed-IP list

#### Scenario: Explicit address is added
- **WHEN** a user requests allow-ip with an explicit IP or CIDR
- **THEN** the launcher adds that address to the SaaS allowed-IP list

#### Scenario: Address already allowed
- **WHEN** a user requests allow-ip for an address already present in the allowed-IP list
- **THEN** the launcher makes no change
- **AND** the launcher reports success

### Requirement: The launcher SHALL dry-run test a SaaS connection
The launcher SHALL verify connectivity to a target SaaS database without transferring data or mutating either database.

#### Scenario: All checks pass
- **WHEN** a user runs the SaaS connection test for a target database
- **AND** the database is reachable over TLS with valid credentials
- **THEN** the launcher reports each check as successful
- **AND** the launcher does not transfer any data

#### Scenario: A check fails
- **WHEN** a user runs the SaaS connection test for a target database
- **AND** a connectivity, allowlist, or credential check fails
- **THEN** the launcher reports the failed check
- **AND** the launcher exits non-zero

### Requirement: The launcher SHALL migrate a deployment into a target SaaS database
The launcher SHALL migrate the local deployment's schemas, tables, table data, and database objects into a SaaS database identified by its database UUID, preserving table distribution keys.

#### Scenario: Migration target is identified by UUID
- **WHEN** a user starts a migration without specifying a target database UUID
- **THEN** the launcher refuses the migration with a non-zero exit

#### Scenario: Objects are recreated before data is loaded
- **WHEN** the launcher migrates to a target SaaS database
- **THEN** the launcher recreates schemas and tables on the target before loading data
- **AND** recreated tables preserve their source distribution keys

#### Scenario: Table data is transferred directly
- **WHEN** the launcher transfers a table's data to the target
- **THEN** the launcher streams rows directly from the source database into the target table over an Exasol-to-Exasol connection

#### Scenario: Dependent objects are recreated in dependency order
- **WHEN** the launcher migrates database objects
- **THEN** the launcher recreates users, roles, connections, views, scripts, and privileges
- **AND** objects are recreated after the objects they depend on

#### Scenario: Non-migratable secrets are not invented
- **WHEN** the launcher migrates connections or users whose secrets cannot be read from the source
- **THEN** the launcher does not fabricate those secrets on the target
- **AND** the launcher reports that the secrets must be set on the target

#### Scenario: Source database is not modified
- **WHEN** the launcher runs a migration
- **THEN** the source database contents are not modified

### Requirement: Migration SHALL support preview and phase selection
The launcher SHALL support previewing a migration without executing it and limiting a migration to objects only or data only.

#### Scenario: Preview reports planned work without executing
- **WHEN** a user runs a migration in preview mode
- **THEN** the launcher reports the object and data-transfer statements it would run
- **AND** neither the source nor the target database is modified

#### Scenario: Phase is limited
- **WHEN** a user runs a migration limited to objects only or data only
- **THEN** the launcher performs only the selected phase

### Requirement: Migration SHALL verify connectivity before transferring data
The launcher SHALL confirm the target SaaS connection succeeds before transferring data, and abort otherwise.

#### Scenario: Connectivity fails before migration
- **WHEN** a user starts a migration
- **AND** the target SaaS connection check fails
- **THEN** the launcher aborts the migration before transferring data

### Requirement: Migration SHALL validate transferred data
The launcher SHALL validate that each migrated table's data matches the source after transfer.

#### Scenario: Row counts match
- **WHEN** the launcher finishes transferring a table
- **THEN** the launcher compares the target row count against the source row count
- **AND** the launcher reports a mismatch as a migration failure
