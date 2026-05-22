## 1. Deployment Contract

- [x] 1.1 Add optional Admin UI connection metadata to the deployment config model and connection info model.
- [x] 1.2 Stop requiring `connection.uiPort` for SQL connection resolution while keeping SQL host and DB port mandatory.
- [x] 1.3 Normalize legacy `connection.uiPort` and `nodes[*].database.uiPort` metadata into the optional Admin UI connection object.
- [x] 1.4 Add unit tests for deployments with explicit Admin UI metadata, legacy UI port metadata, and no Admin UI metadata.

## 2. Presets and Backend Output

- [x] 2.1 Add `admin-ui` to built-in infrastructure presets that expose Administration UI access.
- [x] 2.2 Update AWS, Azure, and Exoscale tofu outputs to write provider-specific `connection.adminUi` metadata.
- [x] 2.3 Ensure the Exasol Local backend omits `connection.adminUi` and `connection.uiPort` metadata.
- [x] 2.4 Add or update tests for local deployment artifact generation and cloud output schema expectations.

## 3. User-Facing Output

- [x] 3.1 Update connection instruction rendering to print the Administration UI section only when Admin UI metadata is present.
- [x] 3.2 Include Admin UI URL, username when known, secret-file reference when secured, and certificate validation guidance from Admin UI metadata.
- [x] 3.3 Preserve existing SQL client, CLI, and shell instruction behavior when Admin UI metadata is absent.
- [x] 3.4 Add unit tests for connection instructions with and without Admin UI metadata.

## 4. Verification

- [x] 4.1 Run Go formatting and focused unit tests for config and deploy packages.
- [x] 4.2 Run targeted integration tests covering preset selection or exported presets affected by the new capability metadata.
- [x] 4.3 Review generated deployment artifacts for one local fake-runner path or fixture to confirm local Admin UI metadata is absent.
