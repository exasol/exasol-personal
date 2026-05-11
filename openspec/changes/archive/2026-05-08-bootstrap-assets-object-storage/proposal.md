## Why

The infrastructure presets currently embed all host file assets from `installation/files/**` and provider `files/**` directly into cloud-init user data. That approach makes user data size grow with every script and unit file change and couples bootstrap transport to inline payload limits instead of using provider object storage that already exists in each target environment.

## What Changes

- Add a provider-managed bootstrap asset distribution flow for AWS, Azure, and Exoscale.
- Create a separate per-deployment bootstrap object store for each provider and destroy it with the deployment.
- Upload all files from `installation/files/**` and provider `assets/infrastructure/<provider>/files/**` into that object store during infrastructure apply.
- Render cloud-init `write_files` entries for those assets using HTTPS `source.uri` values instead of inline `content`.
- Keep `installation/cloudconf/*`, `/etc/exasol_launcher/infrastructure.json`, and `/etc/exasol_launcher/node.json` embedded in cloud-init.
- Use the host `dest_path` with its leading `/` stripped as the object key for uploaded bootstrap assets.
- Restrict AWS bootstrap reads to the deployment S3 VPC endpoint and Azure bootstrap reads to private blob access with signed HTTPS URLs.
- Use direct HTTPS SOS object URLs on Exoscale because SOS does not provide an instance-scoped private-access mechanism analogous to AWS or Azure.

## Capabilities

### New Capabilities
- `bootstrap-asset-distribution`: Provider infrastructure presets distribute node bootstrap file assets through per-deployment object storage and cloud-init HTTPS fetches.

### Modified Capabilities

## Impact

- Affected infrastructure code in `assets/infrastructure/aws`, `assets/infrastructure/azure`, and `assets/infrastructure/exoscale`.
- Cloud-init generation changes in each provider `cloudinit.tf`.
- New provider storage resources, object upload resources, and provider-specific HTTPS URL generation.
- Provider documentation and operational expectations for bootstrap storage lifecycle, access behavior, and the Exoscale private-access limitation.
