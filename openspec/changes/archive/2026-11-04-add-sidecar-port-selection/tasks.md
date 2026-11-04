## 1. Port model

- [x] 1.1 Keep the saved host port of a published port without a `name`, and
  verify with a launcher test that reset keeps it and reports only named
  ports.
- [x] 1.2 Let the local port allocator take a starting port and a reserved
  set. Make database allocation reserve saved sidecar host ports and reject an
  explicit database port saved for a sidecar. Verify with allocator unit tests
  and launcher tests for both `exasol-local-deployment` scenarios.

## 2. Sidecar port selection

- [x] 2.1 Select free host ports when enable saves a catalog template, starting
  at each template `hostPort` and skipping ports saved for other services.
  Verify with tests for an available default, a held default, and a default
  saved for another service.
- [x] 2.2 Add `--ports` to `exasol sidecar enable` with named entries merged
  into the saved definition, updates of an enabled sidecar, and immediate
  reconciliation. Regenerate the CLI reference with `task docs-cli-reference`.
  Verify with tests for explicit ports, enable updating an enabled sidecar,
  invalid entries, and a running sidecar moving to the new port.
- [x] 2.3 Add `exasol sidecar config get`, `set`, and `reset` with text and
  JSON output, reconciliation of running enabled sidecars, the unsaved name
  error, the `exasol config set` apply guidance for a deployment that is not
  running, and host ports in `exasol sidecar list`. Treat a running sidecar's
  own published ports as available during reset. Regenerate the CLI
  reference. Verify with tests for get, reset from the catalog port and from a
  dropped catalog entry's saved port, a stopped deployment applying the port on
  start, an unsaved name, and list reporting saved host ports, and with a
  launcher test that a stopped deployment names the apply command.

## 3. Conflict recovery

- [x] 3.1 Classify sidecar bind failures from container creation and VM port
  forwarding as sidecar port conflicts that keep the saved ports, and add the
  `sidecar config set` recovery commands to text output. Verify with a live
  test that holds a port and sets a running sidecar to it.

## 4. Documentation

- [x] 4.1 Document host port selection, `--ports`, the sidecar config
  commands, and listed host ports in the sidecar user guide and the AI agent
  guide, and update the unreleased sidecar CHANGELOG entry with command
  examples. Verify with `task docs-check`.

## 5. Connection instructions

- [x] 5.1 Print the connection instructions from `exasol start` when the
  database runs and only sidecars fail, followed by the sidecar failure and a
  nonzero exit. Verify with a test of the start failure output.
- [x] 5.2 Rewrite the saved connection instructions after enable, disable,
  config set, and config reset change a saved sidecar of a running deployment.
  Verify with a live test that sets a running MCP sidecar's port and reads the
  saved instructions.
