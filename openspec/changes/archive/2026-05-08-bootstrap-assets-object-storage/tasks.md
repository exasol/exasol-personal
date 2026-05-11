## 1. Shared design and provider contracts

- [x] 1.1 Document the bootstrap asset storage approach in the provider presets and shared preset contract docs.
- [x] 1.2 Define a shared file-to-object-key mapping and cloud-init `write_files.source.uri` rendering pattern for installation and provider overlay files.

## 2. AWS preset

- [x] 2.1 Add a dedicated deployment-scoped bootstrap S3 bucket and object upload resources in `assets/infrastructure/aws`.
- [x] 2.2 Generate HTTPS access URLs for uploaded bootstrap objects, switch AWS cloud-init overlay files from inline `content` to `source.uri`, and restrict bootstrap reads to the deployment S3 VPC endpoint.
- [x] 2.3 Verify bootstrap storage is updated on asset changes and removed on destroy.

## 3. Azure preset

- [x] 3.1 Add a dedicated deployment-scoped bootstrap storage account or container and object upload resources in `assets/infrastructure/azure`.
- [x] 3.2 Generate HTTPS access URLs for uploaded bootstrap objects, switch Azure cloud-init overlay files from inline `content` to `source.uri`, and keep bootstrap blob reads private through signed access.
- [x] 3.3 Verify bootstrap storage is updated on asset changes and removed on destroy.

## 4. Exoscale preset

- [x] 4.1 Add a dedicated deployment-scoped bootstrap SOS bucket and object upload resources in `assets/infrastructure/exoscale`.
- [x] 4.2 Generate HTTPS access URLs for uploaded bootstrap objects and switch Exoscale cloud-init overlay files from inline `content` to `source.uri`.
- [x] 4.3 Verify bootstrap storage is updated on asset changes and removed on destroy, and document the lack of an instance-scoped private-access mechanism for Exoscale SOS.

## 5. Validation

- [x] 5.1 Update provider documentation to describe bootstrap asset storage lifecycle, separation from archive storage, and troubleshooting expectations.
