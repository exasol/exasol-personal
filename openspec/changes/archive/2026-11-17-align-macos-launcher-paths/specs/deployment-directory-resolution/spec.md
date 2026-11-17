## MODIFIED Requirements

### Requirement: Managed deployments SHALL resolve under a platform-specific root
The managed-deployments root SHALL be a platform-conventional, launcher-owned directory distinct from the launcher's configuration and cache locations, rather than one flat path shared across platforms.

#### Scenario: Platform-specific managed-deployments root
- **WHEN** the CLI resolves the managed-deployments root
- **THEN** it uses `~/.exasol/launcher/deployments` on Linux, `~/.exasol/launcher/deployments` on macOS, and `%LOCALAPPDATA%\exasol\launcher\deployments` on Windows
