## ADDED Requirements

### Requirement: Bootstrap assets are available before launcher startup

Each provider preset SHALL retrieve all uploaded installation and provider bootstrap files before starting `exasol_launcher.target`.

#### Scenario: Successful bootstrap asset delivery

- **WHEN** a new node completes cloud-init final-stage processing
- **THEN** every expected bootstrap asset exists at its configured host destination
- **AND** `exasol_launcher.target` is enabled and started only after those files are available

#### Scenario: Bootstrap retrieval occurs after initial network setup

- **WHEN** cloud-init processes a provider bootstrap file configured with a remote source
- **THEN** the file retrieval occurs during the final stage after package and network setup

### Requirement: Missing bootstrap assets fail with actionable diagnostics

The provider bootstrap sequence SHALL verify every expected bootstrap asset before starting the launcher and SHALL stop startup when any expected file is missing.

#### Scenario: Required asset is unavailable after source retries

- **WHEN** a bootstrap source remains unreachable after cloud-init's configured retries
- **THEN** the bootstrap sequence reports the missing destination path
- **AND** it reports that the installation must be rerun
- **AND** it does not start `exasol_launcher.target`

### Requirement: Bootstrap delivery remains within user-data limits

The provider presets SHALL keep launcher file contents in deployment object storage and SHALL add only the deferred-write metadata and validation commands to cloud-init user data.

#### Scenario: Bootstrap reliability is added without embedding launcher files

- **WHEN** cloud-init user data is rendered for any supported provider
- **THEN** uploaded launcher files remain referenced by HTTPS source URIs
- **AND** the rendered user data does not inline those launcher file contents
