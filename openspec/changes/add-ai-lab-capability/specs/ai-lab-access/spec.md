## ADDED Requirements

### Requirement: Infrastructure presets declare AI Lab support
The system SHALL allow infrastructure presets to declare that they support running the Exasol AI Lab on the deployment's infrastructure through an `ai-lab` provided capability.

#### Scenario: Preset advertises AI Lab support
- **WHEN** an infrastructure preset includes `ai-lab` in `compatibility.provides`
- **THEN** the launcher treats that preset as capable of installing and exposing AI Lab for its deployments

#### Scenario: Preset omits AI Lab support
- **WHEN** an infrastructure preset does not include `ai-lab` in `compatibility.provides`
- **THEN** the launcher does not offer AI Lab installation for deployments using that preset

#### Scenario: Local deployment does not support AI Lab
- **WHEN** a deployment uses the Exasol Local infrastructure
- **THEN** AI Lab is not available, because UDFs and BucketFS required by AI Lab are not available locally

### Requirement: AI Lab installation is opt-in
The system SHALL install AI Lab only when the user explicitly requests it, and SHALL NOT install it by default.

#### Scenario: Request AI Lab during install
- **WHEN** a user runs `exasol install` for a supporting preset with the `--with-ai-lab` flag
- **THEN** the launcher installs the database and additionally installs AI Lab on the same infrastructure

#### Scenario: Install without requesting AI Lab
- **WHEN** a user runs `exasol install` without `--with-ai-lab`
- **THEN** the launcher deploys only the database and does not install AI Lab

#### Scenario: Request AI Lab on an unsupported preset
- **WHEN** a user targets a preset whose infrastructure does not provide the `ai-lab` capability
- **THEN** the `--with-ai-lab` option is not offered for that preset, so AI Lab cannot be requested for it

### Requirement: AI Lab runs as a container alongside the database
The system SHALL run AI Lab as the official Exasol AI Lab container, managed by Podman, on the same infrastructure that hosts the database.

#### Scenario: AI Lab container is started on the database host
- **WHEN** AI Lab installation runs for a supporting deployment
- **THEN** the launcher runs the Exasol AI Lab container using Podman on the host that runs the database

#### Scenario: AI Lab uses the latest published image
- **WHEN** the launcher installs the AI Lab container
- **THEN** it uses the latest published `exasol/ai-lab` container image

#### Scenario: AI Lab persists across restarts
- **WHEN** the host running AI Lab is stopped and started, or the deployment is stopped and started
- **THEN** AI Lab becomes available again without re-running installation

### Requirement: AI Lab is pre-configured to connect to the database and BucketFS
The system SHALL pre-configure AI Lab so that it connects to the deployment's Exasol database and BucketFS automatically, requiring no manual configuration steps from the user.

#### Scenario: Database connection is pre-seeded
- **WHEN** AI Lab is installed for a deployment
- **THEN** AI Lab's secure configuration is seeded with the database connection parameters (host, port, user, password, and encryption/certificate settings appropriate for the deployment) so notebooks can connect without manual entry

#### Scenario: BucketFS connection is pre-seeded
- **WHEN** AI Lab is installed for a deployment
- **THEN** a dedicated BucketFS bucket is created for AI Lab if it does not already exist, with a generated write password, and AI Lab's secure configuration is seeded with that bucket's name, user, and password so notebooks can use BucketFS without manual entry

#### Scenario: Database schema is pre-created
- **WHEN** AI Lab is installed for a deployment
- **THEN** a default database schema is created (if absent) and recorded in AI Lab's configuration, so notebooks are ready to use without running the main configuration notebook

#### Scenario: Self-signed certificate is handled
- **WHEN** the deployment's database or BucketFS presents a self-signed certificate
- **THEN** the pre-seeded AI Lab configuration connects successfully without requiring the user to disable certificate validation manually

### Requirement: AI Lab secrets are generated and stored
The system SHALL generate the secrets required to secure and unlock AI Lab and store them with the deployment's secrets.

#### Scenario: Master password is generated and stored
- **WHEN** AI Lab is installed for a deployment
- **THEN** the launcher generates the AI Lab secure-configuration master password and stores it in the deployment's `secrets.json`, alongside the existing database and Admin UI secrets

#### Scenario: Jupyter password is generated and stored
- **WHEN** AI Lab is installed for a deployment
- **THEN** the launcher sets a Jupyter access password for the AI Lab container and stores it in the deployment's `secrets.json`

### Requirement: AI Lab port is exposed and access-restricted
The system SHALL expose the AI Lab Jupyter port through the deployment's network so the user can connect, restricted by the same address allow-list used for other deployment ports.

#### Scenario: AI Lab port is reachable within the allow-list
- **WHEN** a cloud deployment installs AI Lab
- **THEN** the AI Lab port is opened for inbound access subject to the deployment's configured allowed address range

#### Scenario: Access is protected by the Jupyter password
- **WHEN** a client reaches the exposed AI Lab port
- **THEN** access to the Jupyter environment requires the deployment's AI Lab Jupyter password

### Requirement: Deployment metadata contains resolved AI Lab access
The system SHALL represent AI Lab access as optional resolved connection metadata written by the active backend.

#### Scenario: Backend exposes AI Lab
- **WHEN** a backend creates or refreshes a deployment that has AI Lab installed
- **THEN** `deployment.json` contains an AI Lab connection object with the URL that users can open for that concrete deployment

#### Scenario: Backend does not expose AI Lab
- **WHEN** a backend creates or refreshes a deployment that does not have AI Lab installed
- **THEN** `deployment.json` omits the AI Lab connection object

#### Scenario: Local backend does not expose AI Lab
- **WHEN** the Exasol Local backend writes deployment metadata
- **THEN** `deployment.json` omits AI Lab connection metadata

### Requirement: Connection instructions conditionally show AI Lab
The system SHALL show AI Lab connection instructions only when resolved AI Lab metadata is present.

#### Scenario: AI Lab metadata is present
- **WHEN** a running deployment has AI Lab metadata in `deployment.json`
- **THEN** the connection instructions include the AI Lab URL and reference `secrets.json` for the Jupyter and secure-configuration master passwords, without printing the password values (consistent with how database and Admin UI passwords are surfaced)

#### Scenario: AI Lab metadata is absent
- **WHEN** a running deployment has no AI Lab metadata in `deployment.json`
- **THEN** the connection instructions omit the AI Lab section while preserving the SQL connection instructions
