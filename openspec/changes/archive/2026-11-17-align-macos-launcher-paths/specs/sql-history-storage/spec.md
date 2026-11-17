## MODIFIED Requirements

### Requirement: SQL history SHALL persist under a platform-specific, launcher-owned location
The interactive shell's SQL history SHALL be stored under the launcher's own namespace rather than directly in the OS-provided cache directory.

#### Scenario: Platform-specific history location
- **WHEN** the interactive shell resolves the SQL history file path
- **THEN** it uses `~/.exasol/launcher/history/exasol_history` on Linux, `~/.exasol/launcher/history/exasol_history` on macOS, and `%LOCALAPPDATA%\exasol\launcher\history\exasol_history` on Windows
