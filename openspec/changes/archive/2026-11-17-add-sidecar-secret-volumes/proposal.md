## Why

The launcher passes resolved connection values to a sidecar as environment variables. Podman and Docker keep a container's environment in its configuration, so `podman inspect` or `docker inspect` of a sidecar such as the bundled MCP server shows the database password in plain text, and diagnostics a user shares can carry it. The container's applied-definition label is an unsalted hash over the same values, which lets anyone who reads it test password guesses offline.

Related issue: [SPOT-34050](https://exasol.atlassian.net/browse/SPOT-34050).

## What Changes

- Sidecar definitions accept Kubernetes secret volumes: `volumes` beside `container` with a `secret` from a launcher-owned source, and `volumeMounts` on the container. The launcher copies each item into the container as a file before the container starts, so applications that read the password from a file receive it without it entering the container's configuration.
- Environment references keep working as before and remain visible to container inspection; the documentation recommends secret volumes for the password.
- The applied-definition label no longer depends on sensitive values. The launcher recreates a sidecar when the sensitive values it applied no longer match the current ones.
- The documentation describes how a saved sidecar adopts a newer catalog template.

## Capabilities

### Modified Capabilities

- `sidecar-configuration`: Definitions declare secret volumes, whose files container inspection does not reveal.

## Impact

The change affects the sidecar definition format and validation, the local sidecar driver, the container runtimes' creation, copy, and start operations, sidecar reconciliation, the sidecar user and contributor documentation, and the changelog. The catalog, existing saved definitions, and cloud deployments are unchanged.
