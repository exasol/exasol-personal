## ADDED Requirements

### Requirement: Unnamed published ports keep their host port

Host port selection and `--ports` SHALL address published ports by `name`. A
port with a positive `hostPort` and no `name` SHALL keep its saved host port,
SHALL count as taken when the launcher selects other host ports, and SHALL be
omitted from reported `<port-name>:<host-port>` entries.

#### Scenario: Reset keeps an unnamed host port

- **WHEN** a user runs `exasol sidecar config reset <name> ports` for a saved
  sidecar that also publishes a port without a `name`
- **THEN** `sidecars.yaml` keeps that port's host port and the command reports
  only the named ports

### Requirement: Enable selects free host ports for a new definition

When `exasol sidecar enable <name>` saves a catalog template without
`--ports`, the launcher SHALL save a concrete host port for each named
published port. Selection SHALL start at the template's `hostPort` and SHALL
follow the default-first wrapping order of local service port allocation. A
selected port SHALL be available on every loopback address resolved for
`localhost` and SHALL differ from the host ports saved for the database and
for the deployment's other sidecars. When no port is available, enable SHALL
fail, save no definition, and identify the sidecar and port name.

#### Scenario: Default sidecar port is available

- **WHEN** a user enables a catalog sidecar whose template host port is usable
- **THEN** `sidecars.yaml` saves the template host port for that port

#### Scenario: Taken default advances to a free port

- **WHEN** a user enables a catalog sidecar whose template host port is held by
  another process
- **THEN** `sidecars.yaml` saves the first following usable port
- **AND** status reports the sidecar's endpoint on that port

#### Scenario: Selection skips ports saved for other services

- **WHEN** a user enables a catalog sidecar whose template host port is saved
  for the database or another sidecar of the deployment
- **THEN** the launcher saves a different usable host port

### Requirement: Users select sidecar host ports explicitly

`exasol sidecar enable <name> --ports` and
`exasol sidecar config set <name> --ports` SHALL accept comma-separated
`<port-name>:<host-port>` entries, where each port name identifies a published
port of that sidecar. The launcher SHALL save each given host port exactly and
keep the saved host ports of unlisted ports. On an already-enabled sidecar,
`enable --ports` SHALL update its saved host ports as `config set` does. The
launcher SHALL reject, with an error naming the offending entry and saving
nothing, an entry that is not of the form `<port-name>:<host-port>`, names no
published port, repeats a port name, carries a host port outside `1` through
`65535`, or carries a host port saved for the database or another sidecar of
the deployment.

#### Scenario: Explicit host port is saved exactly

- **WHEN** a user runs `exasol sidecar enable <name> --ports <port-name>:<port>`
  for a sidecar that has no saved definition
- **THEN** `sidecars.yaml` saves that host port for the named port

#### Scenario: Enable with ports updates an enabled sidecar

- **WHEN** a user runs `exasol sidecar enable <name> --ports <port-name>:<port>`
  for an enabled sidecar with a different saved host port
- **THEN** `sidecars.yaml` saves the given host port and keeps the rest of the
  saved definition

#### Scenario: Invalid port entry names the offending entry

- **WHEN** a user passes `--ports` with an entry that lacks a port name, an
  unknown port name, an unpublished port, a repeated port name, an
  out-of-range port, or a host port saved for another service
- **THEN** the command fails, names the offending entry, and leaves
  `sidecars.yaml` unchanged

### Requirement: Sidecar config commands manage saved host ports

`exasol sidecar config get <name> [ports]` SHALL report a saved sidecar's host
ports as comma-separated `<port-name>:<host-port>` entries in text and JSON
formats. `exasol sidecar config reset <name> ports` and
`exasol sidecar config reset <name> --all` SHALL select a free host port for
every published port under the selection rules of enable, starting at the
catalog template's host port for that port name, or at the saved host port
when the catalog no longer offers that sidecar or port name. `config set` and
`config reset` SHALL report the resulting values. Each config command SHALL
fail for a name without a saved definition and SHALL tell the user to enable
the sidecar first.

#### Scenario: Config get reports saved host ports

- **WHEN** a user runs `exasol sidecar config get <name>` for a saved sidecar
- **THEN** text and JSON output report its saved host ports

#### Scenario: Config reset selects automatic host ports

- **WHEN** a user runs `exasol sidecar config reset <name> ports`
- **THEN** each published port saves the first usable host port from its
  catalog template's host port, and the command reports the saved ports

#### Scenario: Config reset for a sidecar the catalog dropped

- **WHEN** a user resets the ports of a saved sidecar that the catalog no
  longer offers
- **THEN** selection starts at each port's saved host port

#### Scenario: Config command for an unsaved sidecar fails

- **WHEN** a user runs a sidecar config command for a name without a saved
  definition
- **THEN** the command fails and tells the user to run
  `exasol sidecar enable <name>`

## MODIFIED Requirements

### Requirement: Users can discover supported sidecars

`exasol sidecar list` SHALL report catalog names, descriptions, architecture
support, enablement, and host ports for the selected deployment in text and
JSON formats. Host ports SHALL be reported as comma-separated
`<port-name>:<host-port>` entries: the saved host ports for a sidecar with a
saved definition and the catalog template's host ports otherwise. Without an
initialized deployment it SHALL report the catalog with the current machine's
architecture support and no entry enabled. New enable requests SHALL resolve a
supported catalog name.

#### Scenario: Catalog listing describes deployment availability

- **WHEN** the user lists sidecars for a deployment with an enabled entry
- **THEN** the output identifies the catalog entries, architecture support,
  and which entry is enabled in both text and JSON formats

#### Scenario: Listing reports host ports by port name

- **WHEN** the user lists sidecars for a deployment whose saved catalog
  sidecar has a host port other than its template's
- **THEN** that entry reports its saved host ports as
  `<port-name>:<host-port>` entries in both text and JSON formats

#### Scenario: Listing reflects the selected deployment's type

- **WHEN** the user lists sidecars for a deployment that cannot run them
- **THEN** every entry reports itself unsupported and says why

#### Scenario: Catalog listing answers before a deployment exists

- **WHEN** the user lists sidecars without an initialized deployment
- **THEN** the output identifies the catalog entries with the current machine's
  architecture support and reports no entry as enabled

#### Scenario: Unsupported selection explains the available choices

- **WHEN** the user enables an unknown name or an architecture-incompatible
  catalog entry
- **THEN** the command fails with the selection error and supported choices
