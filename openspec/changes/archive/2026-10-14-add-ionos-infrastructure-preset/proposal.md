## Why

Exasol Personal deploys to AWS, Azure, Exoscale, and STACKIT, but not to IONOS Cloud. Users who choose IONOS for German or EU data residency have to build and maintain their own infrastructure preset (#425).

## What Changes

- Add a built-in `ionos` infrastructure preset that works with the `ubuntu` installation preset: `exasol install ionos`.
- Authenticate with an API token from `IONOS_TOKEN` only.
- Size nodes as `<cores>x<ram_gb>` dedicated core servers through `--instance-type`, with `--cluster-size`, `--location`, `--cpu-family`, `--disk-type`, `--os-volume-size`, and `--data-volume-size`.
- Create one data center per deployment, or deploy into an existing one with `--datacenter-id`.
- Keep node addresses stable across `exasol stop` and `exasol start` through reserved IPs.
- Document IONOS Cloud setup and configuration in the user documentation, and list IONOS alongside the other providers.

## Capabilities

### New Capabilities

- `ionos-cloud-deployment`: Deploying, stopping, starting, and destroying Exasol Personal in an IONOS Cloud contract.

### Modified Capabilities

## Impact

- New preset in `assets/infrastructure/ionos`, embedded automatically with the other built-in presets.
- Launcher help text, the embedded preset test, and launcher tests that name the built-in presets.
- User documentation, `README.md`, and `CHANGELOG.md`.
- Out of scope: IONOS in the live test suite and CI deployment matrix, which need IONOS credentials in CI, and an IONOS collector for `tools/cleanup`.
