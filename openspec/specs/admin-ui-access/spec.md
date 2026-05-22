# admin-ui-access Specification

## Purpose
TBD - created by archiving change add-admin-ui-capability. Update Purpose after archive.
## Requirements
### Requirement: Infrastructure presets declare Admin UI support
The system SHALL allow infrastructure presets to declare that they support Administration UI exposure through an `admin-ui` provided capability.

#### Scenario: Preset advertises Admin UI support
- **WHEN** an infrastructure preset includes `admin-ui` in `compatibility.provides`
- **THEN** the launcher treats that preset as capable of exposing Administration UI access

#### Scenario: Preset omits Admin UI support
- **WHEN** an infrastructure preset does not include `admin-ui` in `compatibility.provides`
- **THEN** the launcher does not assume that Administration UI access is available for deployments using that preset

### Requirement: Deployment metadata contains resolved Admin UI access
The system SHALL represent Administration UI access as optional resolved connection metadata written by the active backend.

#### Scenario: Backend exposes Admin UI
- **WHEN** a backend creates or refreshes a deployment whose infrastructure exposes Administration UI access
- **THEN** `deployment.json` contains an Admin UI connection object with the URL that users can open for that concrete deployment

#### Scenario: Backend does not expose Admin UI
- **WHEN** a backend creates or refreshes a deployment whose infrastructure does not expose Administration UI access
- **THEN** `deployment.json` omits the Admin UI connection object

#### Scenario: Local backend does not expose Admin UI
- **WHEN** the Exasol Local backend writes deployment metadata
- **THEN** `deployment.json` omits Admin UI connection metadata

#### Scenario: Cloud backend exposes provider-specific Admin UI
- **WHEN** a cloud infrastructure preset writes deployment metadata for a deployment that supports Admin UI
- **THEN** the Admin UI connection URL uses the provider-specific reachable host or address for that deployment

### Requirement: Connection instructions conditionally show Admin UI
The system SHALL show Administration UI connection instructions only when resolved Admin UI metadata is present.

#### Scenario: Admin UI metadata is present
- **WHEN** a running deployment has Admin UI metadata in `deployment.json`
- **THEN** the connection instructions include the Administration UI URL and available login information

#### Scenario: Admin UI metadata is absent
- **WHEN** a running deployment has no Admin UI metadata in `deployment.json`
- **THEN** the connection instructions omit the Administration UI section while preserving SQL connection instructions

### Requirement: Legacy UI port metadata remains readable
The system SHALL preserve compatibility with existing deployment metadata that only contains UI port fields.

#### Scenario: Legacy connection UI port exists
- **WHEN** `deployment.json` contains a connection UI port but no Admin UI connection object
- **THEN** the launcher derives equivalent Admin UI metadata during deployment info normalization

#### Scenario: Legacy node UI port exists
- **WHEN** `deployment.json` contains node database UI port metadata but no explicit connection metadata
- **THEN** the launcher derives connection metadata that includes Administration UI access

