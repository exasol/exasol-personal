## MODIFIED Requirements

### Requirement: The cache configuration file SHALL live at a platform-specific configuration location
The resource cache's configuration SHALL be one `resources.yaml` file per user, located under the platform's launcher configuration root.

#### Scenario: Platform-specific cache config file location
- **WHEN** the launcher resolves the cache configuration file path
- **THEN** it uses `${XDG_CONFIG_HOME:-~/.config}/exasol/launcher/resources.yaml` on Linux, `~/.exasol/launcher/resources.yaml` on macOS, and `%APPDATA%\exasol\launcher\resources.yaml` on Windows
