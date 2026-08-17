## ADDED Requirements

### Requirement: Config set SHALL report a clear error when configurable options cannot be loaded
The `config set` command SHALL resolve the target deployment's configurable infrastructure
options before parsing supplied option flags. When it cannot load those options because no
initialized deployment directory can be resolved, it SHALL fail with a clear, actionable error
that identifies the resolved deployment directory and directs the user to initialize it or to
supply `--deployment-dir`, rather than reporting supplied infrastructure options as unknown
flags.

#### Scenario: Config set names the unresolved deployment directory
- **WHEN** a user runs `exasol config set <infrastructure-options>` and no initialized
  deployment directory can be resolved (for example `--deployment-dir` is omitted and the
  current directory is not a deployment, or the target directory is not initialized)
- **THEN** the command fails with an error identifying the resolved deployment directory
- **AND** the error tells the user to initialize it with `exasol init` or `exasol install`, or
  to pass `--deployment-dir` pointing to an existing deployment
- **AND** the error does not report the supplied infrastructure options as unknown flags

#### Scenario: Config set help renders without a resolvable deployment
- **WHEN** a user runs `exasol config set --help` and the deployment's configurable options
  cannot be loaded
- **THEN** the command prints help without failing

#### Scenario: Config set still rejects genuinely unknown options
- **WHEN** a user runs `exasol config set` against an initialized deployment and supplies an
  option that the deployment's presets do not define
- **THEN** the command fails reporting that option as unknown
