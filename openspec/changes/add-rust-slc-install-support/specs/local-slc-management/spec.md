## MODIFIED Requirements

### Requirement: Install an official script language container by alias

`exasol slc install <alias>` SHALL resolve the alias against the official SLC catalog and
make the corresponding language available in the local deployment, without the user running
any SQL. The alias `rust` is reserved: it is not a catalog entry, and the command SHALL
dispatch it to the Rust language container install instead of a catalog lookup. Every other
aspect of the command — flags, restart confirmation, deployment resolution, and output — SHALL
be the same for `rust` as for a catalog alias.

#### Scenario: Install a known alias

- **WHEN** `exasol slc install python3` is run against a local deployment
- **THEN** the resolved official SLC is mounted into the database
- **AND** the command reports success only after the database is ready with the SLC active

#### Scenario: Alias matching is case-insensitive

- **WHEN** `exasol slc install PYTHON3` and `exasol slc install python3` are run
- **THEN** both resolve to the same catalog entry

#### Scenario: The reserved `rust` alias is dispatched, not looked up in the catalog

- **WHEN** `exasol slc install rust` is run against a local deployment
- **THEN** the command installs the Rust language container instead of failing as an unknown alias
- **AND** it accepts the same flags as any other `slc install <alias>` invocation, and offers no
  `--source` flag

#### Scenario: Unknown alias is rejected

- **WHEN** `exasol slc install nodejs` is run and `nodejs` is not in the catalog
- **THEN** the command fails with an error listing the valid aliases
- **AND** no deployment state is changed and no restart occurs

#### Scenario: Unsupported on non-local backends

- **WHEN** `exasol slc install python3` is run against a non-local deployment
- **THEN** the command fails with a clear "unsupported" message
- **AND** no deployment state is changed

#### Scenario: Not-yet-deployed deployment is refused

- **WHEN** an SLC change (`install`, `update`, or `remove`) is run against a deployment that is initialized but not deployed yet
- **THEN** the command fails asking the user to run `exasol deploy` first
- **AND** no deployment state is changed and no SLC is recorded

#### Scenario: Installing an already-installed image is a no-op

- **WHEN** `exasol slc install python3` is run and the resolved image is already installed
- **THEN** the command reports it is already installed and up to date
- **AND** no deployment state is changed and no restart occurs

### Requirement: Update an installed SLC to the catalog's current version

`exasol slc update <alias>` SHALL re-resolve the alias against the catalog and compare the
resolved image with the installed one. When the resolved image is unchanged the command
SHALL be a no-op with no restart; when it has changed the command SHALL replace the installed
SLC and apply it through a database restart. Update SHALL NOT order versions or guard against
"older" images — rollback is out of scope, so it installs whatever the catalog resolves to.
`exasol slc update rust` SHALL follow the same dispatch as `exasol slc install rust`: it
re-resolves the latest Rust language container release instead of a catalog entry, and is
likewise a no-op when the resolved release is unchanged.

#### Scenario: Update with no catalog change is a no-op

- **WHEN** `exasol slc update python3` is run and the resolved image matches the installed one
- **THEN** the command reports it is already up to date
- **AND** no deployment state is changed and no restart occurs

#### Scenario: Update applies a changed image

- **WHEN** `exasol slc update python3` is run and the catalog resolves to a different image
- **THEN** the installed SLC is replaced with the newly resolved one
- **AND** success is reported only after the database is ready with the new image active

#### Scenario: Update of a not-installed SLC

- **WHEN** `exasol slc update python3` is run and `python3` is not installed
- **THEN** the command reports that nothing is installed for that alias
- **AND** no restart occurs

#### Scenario: Update of the reserved `rust` alias re-resolves the latest release

- **WHEN** `exasol slc update rust` is run
- **THEN** the latest Rust language container release is resolved again
- **AND** the container is reinstalled when the release has moved, and reported as up to date when
  it has not

## ADDED Requirements

### Requirement: The `rust` alias resolves the latest Rust language container release

`exasol slc install rust` and `exasol slc update rust` SHALL resolve the latest release of
`exasol-labs/language-container-rs` and install the release asset that matches the host CPU
architecture, under the fixed alias `RUST` with the language identifier `rust`. Because the
release asset file names contain the version, the launcher MUST read the release tag from
`https://api.github.com/repos/exasol-labs/language-container-rs/releases/latest` rather than
relying on a fixed-name `releases/latest/download` URL. The launcher SHALL map `amd64` to
`lc-rust-<version>.tar.gz` and `arm64` to `lc-rust-<version>-aarch64.tar.gz`, and SHALL reject any
other architecture before contacting the network. A failure to resolve the release or to download
the asset SHALL leave the deployment unchanged.

The download SHALL be protected by TLS, by the existing `https`-only redirect restriction, and by
the existing archive validation, which requires an executable `exaudf/exaudfclient` in the
tarball. The launcher SHALL NOT verify the asset against a checksum or signature, because
`latest` is a moving target; the trust bar is therefore the same as
`exasol slc custom install --source <https-url>` and lower than the version- and sha256-pinned
official SLC catalog.

#### Scenario: Install resolves the latest release for an amd64 host

- **WHEN** `exasol slc install rust` is run on an `amd64` host
- **THEN** the launcher reads the latest release tag from the GitHub releases API
- **AND** it installs the `lc-rust-<version>.tar.gz` asset of that release under the alias `RUST`
  with language `rust`

#### Scenario: Install selects the aarch64 asset on an arm64 host

- **WHEN** `exasol slc install rust` is run on an `arm64` host
- **THEN** the `lc-rust-<version>-aarch64.tar.gz` asset of the latest release is installed

#### Scenario: An unsupported architecture fails without a network call

- **WHEN** `exasol slc install rust` is run on a host whose architecture is neither `amd64` nor
  `arm64`
- **THEN** the command fails naming the supported architectures
- **AND** no request is made to the GitHub releases API and no deployment state is changed

#### Scenario: A failed release lookup changes nothing

- **WHEN** the latest release cannot be resolved — the API is unreachable, rate-limited, or returns
  no usable tag
- **THEN** the command fails before staging or downloading anything
- **AND** the deployment and the database are unchanged

#### Scenario: The downloaded asset is validated by shape, not by checksum

- **WHEN** the resolved asset is downloaded and does not contain an executable
  `exaudf/exaudfclient`
- **THEN** the command fails before changing the deployment or the database
- **WHEN** the resolved asset is a valid container
- **THEN** it is installed without being compared against a pinned checksum or signature

#### Scenario: Reinstalling an unchanged release is a no-op

- **WHEN** `exasol slc install rust` or `exasol slc update rust` is run and the resolved release
  has the same content as the container already installed and active under `RUST`
- **THEN** the command makes no change, reports that nothing was done, and does not restart the
  database

### Requirement: An installed `rust` container is managed as a custom SLC

An installed Rust language container SHALL be recorded in launcher state as a custom SLC under the
alias `RUST`, so that `exasol slc list`, `exasol slc remove rust`, restart confirmation,
`--auto-approve`, `--no-restart`, deferred activation, and image and package reclamation apply to
it through the existing generic custom-SLC behavior, with no `rust`-specific handling. A `RUST`
alias that was created outside the launcher — for example by editing `SCRIPT_LANGUAGES` through
SQL — is not tracked in launcher state and SHALL NOT be expected to be detected or preserved.

#### Scenario: List and remove need no `rust`-specific behavior

- **WHEN** the Rust container is installed and `exasol slc list` is run
- **THEN** it is listed as a custom container with alias `RUST`, language `rust`, its status, and
  its source
- **WHEN** `exasol slc remove rust` is run
- **THEN** the container is deactivated and its staged package deleted, like any other custom SLC

#### Scenario: Restart handling is inherited

- **WHEN** `exasol slc install rust` is run on a running deployment
- **THEN** the restart is confirmed before anything is downloaded, `--auto-approve` skips the
  prompt, and `--no-restart` records the container for activation on the next start

#### Scenario: An untracked `RUST` alias is taken over

- **WHEN** a deployment has a `RUST` entry in `SCRIPT_LANGUAGES` that the launcher did not install
- **AND** `exasol slc install rust` is run
- **THEN** the alias is taken over by the installed container without a warning, because an
  untracked alias is neither an official catalog alias nor recorded launcher state
