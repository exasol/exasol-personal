# bootstrap-asset-distribution Specification

## Purpose
Reduce provider cloud-init payload size by distributing installation and infrastructure file overlays through provider-managed HTTPS object storage while keeping bootstrap metadata embedded.
## Requirements
### Requirement: Provider presets SHALL externalize bootstrap file overlays through deployment object storage
Each infrastructure preset for AWS, Azure, and Exoscale SHALL upload all files from the selected installation preset `files/**` tree and the provider preset `files/**` tree into a deployment-scoped bootstrap object store during infrastructure provisioning, using the host `dest_path` with its leading `/` stripped as the object key.

#### Scenario: Upload bootstrap assets for a deployment
- **WHEN** a deployment is created with any supported provider preset
- **THEN** the preset uploads every installation overlay file and provider overlay file needed on the host into a provider-managed bootstrap object store for that deployment

#### Scenario: Refresh uploaded assets after file content changes
- **WHEN** a bootstrap asset file changes and the infrastructure is applied again
- **THEN** the corresponding uploaded object and the cloud-init reference for that file are updated to match the new content

### Requirement: Cloud-init SHALL fetch bootstrap file overlays over HTTPS sources
For uploaded bootstrap assets, provider presets SHALL render cloud-init `write_files` entries that use HTTPS `source.uri` values rather than inline file `content`.

#### Scenario: Fetch installation and infrastructure files during first boot
- **WHEN** a node boots from a deployment created by a supported provider preset
- **THEN** cloud-init retrieves each bootstrap overlay file from a provider-generated HTTPS source URI and writes it to the expected target path on the host

#### Scenario: Restrict AWS and Azure bootstrap storage to deployment-local access
- **WHEN** a deployment is created with the AWS or Azure preset
- **THEN** the bootstrap object store is not publicly readable from the internet
- **AND** the generated HTTPS source URIs remain usable from the deployed instances through provider-managed private access controls

#### Scenario: Use direct HTTPS bootstrap object URLs on Exoscale
- **WHEN** a deployment is created with the Exoscale preset
- **THEN** the preset uses HTTPS SOS object URLs for bootstrap assets
- **AND** the provider design artifacts document that Exoscale SOS does not provide an instance-scoped private-access mechanism analogous to the AWS and Azure presets

#### Scenario: Preserve bootstrap metadata as embedded content
- **WHEN** cloud-init is rendered for a deployment
- **THEN** the preset still embeds `installation/cloudconf/*`, `/etc/exasol_launcher/infrastructure.json`, and `/etc/exasol_launcher/node.json` directly in cloud-init instead of moving them to object storage

### Requirement: Bootstrap object storage SHALL be lifecycle-bound to the deployment
Each provider preset SHALL create a dedicated bootstrap object store for the deployment, keep it separate from archive storage, and delete its uploaded assets and backing storage when the deployment is destroyed.

#### Scenario: Create separate bootstrap storage
- **WHEN** infrastructure for a deployment is created
- **THEN** the provider preset provisions a dedicated bootstrap storage resource that is distinct from any archive storage resource used by the deployment

#### Scenario: Clean up bootstrap storage on destroy
- **WHEN** the deployment is destroyed through the normal infrastructure workflow
- **THEN** the uploaded bootstrap assets and their backing object store are removed as part of that destroy operation
