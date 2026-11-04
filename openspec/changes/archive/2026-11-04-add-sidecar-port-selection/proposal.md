## Why

A sidecar publishes its catalog host port as-is, so enabling it fails when
another process already holds that port, and changing the port means editing
`sidecars.yaml` by hand. The database already selects a free port
automatically and accepts an explicit one through `--ports`. Sidecars need the
same choice.

## What Changes

- `exasol sidecar enable <name>` selects a free host port for each published
  port when it saves a catalog template. The scan starts at the catalog host
  port and follows the database's order.
- `exasol sidecar enable <name> --ports <port-name>:<port>,...` saves the
  given host ports exactly. On an already-enabled sidecar it updates the saved
  ports.
- New `exasol sidecar config get <name>`, `config set <name> --ports`, and
  `config reset <name>` commands manage a saved sidecar's host ports, mirroring
  `exasol config`. Every `--ports` entry names its port, and reset selects
  free ports again.
- A port change on an enabled sidecar of a running deployment is applied
  immediately, as enable does. A bind conflict keeps the saved ports and tells
  the user how to choose another port.
- Automatic selection for the database and for sidecars skips host ports
  already saved for other services of the deployment, and an explicit host
  port saved for another service is rejected.
- A published sidecar port without a `name` keeps its saved host port.
- `exasol sidecar list` reports each sidecar's host ports by port name.
- A sidecar change on a running deployment rewrites the saved connection
  instructions, and `exasol start` prints the connection instructions when
  the database runs and only sidecars fail.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `sidecar-configuration`: enable and the new sidecar config commands select,
  save, and report host ports of named published ports, and list reports
  host ports.
- `sidecar-lifecycle`: a port change on a running enabled sidecar is
  reconciled immediately, and a bind conflict reports recovery commands.
- `exasol-local-deployment`: automatic database port selection skips host
  ports saved for sidecars.
- `deployment-info-reporting`: saved connection instructions follow sidecar
  changes, and start prints them when only sidecars fail.

## Impact

- **Code:** the local port allocator, the sidecar enable flow, sidecar port
  validation, and new `sidecar config` subcommands.
- **Docs:** the sidecar user guide, the AI agent guide, the generated CLI
  reference, and the unreleased sidecar changelog entry.
- **Users:** enabling a sidecar succeeds when its default port is taken, and
  users can move a sidecar to another port with one command.
