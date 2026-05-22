## Why

Some deployment types can expose the Exasol Administration UI, but the launcher currently treats the UI port as an always-present database connection detail. We need an explicit capability and runtime artifact contract so each backend can expose the correct Admin UI URL only when the selected infrastructure supports it.

## What Changes

- Add an `admin-ui` infrastructure capability that presets can provide when their backend can expose the Administration UI.
- Make Admin UI connection details optional in deployment artifacts.
- Have each supported backend resolve Admin UI access into concrete deployment metadata; cloud tofu presets write provider-specific URLs while local deployments omit Admin UI metadata.
- Update connection instructions and information output to show Admin UI access only when deployment metadata contains an Admin UI endpoint.
- Preserve existing SQL connection behavior and shell support behavior.

## Capabilities

### New Capabilities
- `admin-ui-access`: Conditional Administration UI exposure through preset capabilities and backend-resolved deployment metadata.

### Modified Capabilities

## Impact

- Preset compatibility metadata and validation.
- `deployment.json` connection schema and compatibility normalization.
- Cloud infrastructure preset outputs for AWS, Azure, and Exoscale.
- Exasol Local backend artifact generation.
- Connection instruction rendering and tests for deployments with and without Admin UI metadata.
