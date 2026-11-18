## Context

A sidecar's saved definition names its connection values through `valueFrom.secretKeyRef` on the launcher-owned `exasol-database` source, and the launcher resolves them when it creates the container. Resolved values reach the engine through an environment file that is removed after creation, but the engine copies every variable into the container's configuration. The applied-definition label is a SHA-256 over the resolved container, password included, and reconciliation recreates a container when that label changes, which is how a rotated password reaches a running sidecar.

Saved definitions stay authoritative when the catalog changes. Neither engine can set an environment variable that inspection does not show: Docker offers secrets only to Swarm services, and Podman 4.9 shows environment secrets in `podman inspect`. Since release 2.3, the `exasol/mcp-server` image has no shell and runs as a non-root user.

## Decisions

1. **The definition declares how it receives a value.**

   An environment reference receives the value in the container's configuration, as before. A secret volume receives it as a file that the application reads itself. The launcher cannot tell from an image whether its application reads files, so it follows the definition. Exporting copied files into the environment through a `/bin/sh` entrypoint was rejected because it fails for images without a shell, including current MCP server releases.

2. **Secret volumes follow the Kubernetes pod shape.**

   `volumes` sit beside `container`, as they do in a pod, and the container mounts them through `volumeMounts`. A volume supports a `secret` with `secretName`, `items` of `key` and relative `path`, and `optional`; without items every key becomes a file named after it. This keeps saved definitions usable with Kubernetes and needs no launcher-specific vocabulary.

3. **Files are copied into the created container before it starts.**

   The launcher creates the container without starting it, copies an archive of the items to their mounts through the engine's copy command, and then starts it. Files are readable by every user in the container, because only that container sees them, and they live as long as the container does. Files and their directories are read-only to every user except `root`; `readOnly` is accepted for Kubernetes compatibility, because enforcing it against `root` would need an engine volume filled by a helper container, and protection from the container's own processes is not a goal. Only the mount and its subdirectories receive archive entries, so directories the image already has keep their permissions. A read-only mount of owner-only host files was rejected because only the container's root user could read it and because it leaves host files to clean up when creation fails.

4. **The label omits sensitive values, and an applied record tracks them.**

   The applied-definition label replaces sensitive environment values with a placeholder and covers the volume declarations but not their contents. After a container starts, the launcher keeps an owner-only record of a digest of the sensitive environment values and file contents it received, removed with the container and when its sidecar is no longer desired. Reconciliation and status treat a container as current only when its label matches and the record matches the current values. Labels of definitions without sensitive values or volumes keep their previous form, so an upgrade recreates only sidecars that use the password. A keyed hash in the label was rejected because the key would need storage and rotation of its own.

5. **Saved definitions move to secret volumes when users re-enable them.**

   Existing definitions keep their environment references, and the documentation names `exasol sidecar disable <name> --remove` and `exasol sidecar enable <name>` to adopt a newer catalog template. The bundled MCP entry moves to a secret volume once the MCP server reads its password from a file, which is outside this change.

## Risks

- [Sidecars that pass the password as an environment variable stay inspectable] → The user documentation recommends secret volumes and describes the migration.
- [The copy and the start are separate engine commands] → A container whose copy or start fails is not running, so the next reconciliation replaces it.
- [The upgrade recreates running sidecars that reference the password] → Status reports reconciliation as pending until the next start.
