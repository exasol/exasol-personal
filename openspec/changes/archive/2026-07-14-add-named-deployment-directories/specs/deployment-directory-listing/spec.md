## ADDED Requirements

### Requirement: CLI SHALL list known deployment directories
`exasol deployments list` SHALL enumerate the deployment directories under `~/.exasol/personal/deployments/`, including the default deployment directory and every named deployment directory.

#### Scenario: Listing shows the default deployment directory
- **WHEN** a user runs `exasol deployments list`
- **AND** `~/.exasol/personal/deployments/default` exists
- **THEN** the output includes an entry for `default` with its resolved path

#### Scenario: Listing shows named deployment directories
- **WHEN** a user runs `exasol deployments list`
- **AND** one or more named deployment directories exist under `~/.exasol/personal/deployments/`
- **THEN** the output includes one entry per named deployment directory, with its name and resolved path

#### Scenario: Listing is empty when no deployment directories exist
- **WHEN** a user runs `exasol deployments list`
- **AND** `~/.exasol/personal/deployments/` does not exist or contains no deployment directories
- **THEN** the command succeeds
- **AND** the output indicates that no deployment directories exist

#### Scenario: Non-directory entries are ignored
- **WHEN** a user runs `exasol deployments list`
- **AND** `~/.exasol/personal/deployments/` contains a non-directory entry (a file or symlink)
- **THEN** the output does not include an entry for it
- **AND** the command does not fail because of it

#### Scenario: Listing is sorted alphabetically by name
- **WHEN** a user runs `exasol deployments list`
- **AND** more than one deployment directory exists under `~/.exasol/personal/deployments/`
- **THEN** the entries appear in alphabetical order by name, regardless of filesystem iteration order

### Requirement: Listing SHALL report initialization status and preset identity
For each listed deployment directory, `exasol deployments list` SHALL report whether it is initialized, and when initialized, its infrastructure and installation preset identity.

#### Scenario: Initialized deployment shows preset identity
- **WHEN** a user runs `exasol deployments list`
- **AND** a listed deployment directory is initialized with an infrastructure preset and an installation preset
- **THEN** that entry reports status `initialized`
- **AND** that entry reports the infrastructure and installation preset identity

#### Scenario: Uninitialized deployment is reported without failing the listing
- **WHEN** a user runs `exasol deployments list`
- **AND** a listed deployment directory exists but is not initialized
- **THEN** that entry reports status `not_initialized`
- **AND** the command still succeeds and lists the remaining entries

#### Scenario: A legacy-marker deployment is reported as initialized
- **WHEN** a user runs `exasol deployments list`
- **AND** a listed deployment directory is recognized as an initialized deployment only through its legacy marker file, using the same recognition check the CLI uses elsewhere for current-working-directory detection
- **THEN** that entry reports status `initialized`, consistent with how the rest of the CLI treats that directory

#### Scenario: Listing a deployment directory does not modify it
- **WHEN** a user runs `exasol deployments list`
- **AND** a listed deployment directory is initialized but its preset identity is not yet persisted in its state file
- **THEN** that entry reports a derived preset identity
- **AND** the deployment directory's state file is not modified as a result of running `exasol deployments list`

### Requirement: Listing SHALL indicate the currently active deployment directory
`exasol deployments list` SHALL indicate which listed deployment directory would be selected as the active deployment directory if a deployment command were run with no `--deployment-dir` or `--deployment`/`-d` flag from the current working directory.

#### Scenario: Active entry reflects current working directory
- **WHEN** a user runs `exasol deployments list` from within a recognized deployment directory that is listed
- **THEN** that entry is marked as active
- **AND** no other entry is marked as active

#### Scenario: Active entry reflects the default when not inside a recognized directory
- **WHEN** a user runs `exasol deployments list` outside any recognized deployment directory
- **THEN** the `default` entry is marked as active if it exists
- **AND** no other entry is marked as active

#### Scenario: No listed entry is marked active when the active directory is outside the listed tree
- **WHEN** a user runs `exasol deployments list` from within a recognized deployment directory that is not under `~/.exasol/personal/deployments/` (for example, a directory previously selected via `--deployment-dir <path>`)
- **THEN** no listed entry is marked as active

### Requirement: Listing SHALL support JSON output
`exasol deployments list` SHALL support a `--json` flag that emits the same information as structured JSON instead of human-readable text.

#### Scenario: JSON output includes all listed fields
- **WHEN** a user runs `exasol deployments list --json`
- **THEN** stdout is valid JSON
- **AND** each entry includes name, path, status, preset identity when initialized, and whether it is active
