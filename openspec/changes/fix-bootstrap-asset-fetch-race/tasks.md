## 1. Deferred bootstrap delivery

- [x] 1.1 Mark installation and provider bootstrap object-storage files for deferred cloud-init writing in AWS, Azure, Exoscale, and STACKIT.
- [x] 1.2 Generate an inline preflight manifest from the provider bootstrap asset map and report missing destination paths before launcher startup.
- [x] 1.3 Move launcher startup from the shared early `runcmd` sequence to a final-stage shell part after deferred writes complete.

## 2. Validation

- [x] 2.1 Apply the deferred bootstrap sequence consistently across all supported providers.
- [x] 2.2 Run Terraform formatting, Terraform linting, and whitespace validation.
- [ ] 2.3 Verify successful and failed bootstrap paths in provider deployment tests.
