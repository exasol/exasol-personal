## Why

`exasol sidecar status` prints a host and a port, such as
`Endpoint: 127.0.0.1:4896`. That address is incomplete for the MCP server, which
accepts connections at `http://127.0.0.1:4896/mcp`, so a user who pastes it into
a client gets a connection error and the user documentation has to tell readers
to add the missing scheme and path themselves.

A Kubernetes container port carries no scheme or path, so the saved container
definition cannot describe the URL. Kubernetes keeps the same separation: its
application protocol hint lives on `ServicePort`, not on `ContainerPort`.

## What Changes

- A sidecar definition gains an `endpoints` declaration beside `container`, in
  the catalog and in a deployment's `sidecars.yaml` alike. Each entry names one
  of the container's ports and gives the scheme and optional path that the port
  speaks.
- Enabling saves the declarations under `sidecars.<name>`, so an enabled sidecar
  keeps reporting its URL after the catalog entry changes or is removed.
- `exasol sidecar status` reports a complete URL for a published port that has a
  declaration, in both text and JSON output. The JSON endpoint object gains a
  `url` field. A published port with no declaration reports only its host and
  port.
- The MCP catalog entry declares `http` and `/mcp` on its `mcp` port, so status
  reports `http://127.0.0.1:4896/mcp`.
- A declaration naming a port the sidecar keeps internal is accepted and reports
  nothing, because such a port has no host address a URL could point at.

## Capabilities

### Modified Capabilities

- `sidecar-configuration`: adds the endpoint declaration, its validation, and
  its persistence on the saved definition.
- `sidecar-networking`: status reports the complete URL for a published port
  that declares one, resolved against the effective published host address.
- `sidecar-lifecycle`: status outcomes name endpoints rather than IP and port
  endpoints, so the wording covers a reported URL.

## Impact

- The sidecar package gains the `Endpoint` type, its validation, and its place
  on the shared definition a catalog entry and a saved sidecar both carry.
- Sidecar inspection matches an observed binding back to its declared port and
  builds the URL the status result carries into text and JSON output.
- The embedded catalog, the user and developer sidecar pages, and the changelog.
- The definition hash covers the container alone, so editing a scheme or path
  changes the reported URL without requiring a restart.
