## Context

The Ubuntu cloud preset already installs `@license:personal-unlimited`. Deployment metadata contains all nodes, but the resolved connection has one host. The Go driver accepts comma-separated hosts with one shared port, shuffles the hosts, and retries transport failures before login. Its default per-host handshake timeout can exceed the launcher total deadline, so the launcher bounds individual transport attempts through its public transport/login APIs. A rejected login ends the attempt. Cloud presets distribute one certificate across the cluster.

## Goals / Non-Goals

Deliver SPOT-33856 and SPOT-33857 with shared endpoint resolution and separate presentation, transport-deadline, and connection-selection commits. Preserve the old singular host fields and the legacy connection path. License migration and publishing are outside scope.

## Decisions

- Add optional `databaseConnection.allowAllNodes` metadata to the saved installation manifest. The copied preset is the source of truth, with absence selecting the legacy primary endpoint. Avoid timestamps, launcher-version cutoffs, script parsing, and database license queries. Keep this out of workflow JSON because older launchers can discard unknown fields when rewriting it. Older manifest readers tolerate unknown keys.
- Resolve endpoints once in the configuration layer for both `connect` and `info`. Each endpoint identifies its node, public host, database port, and whether the deployment permits connections there. List public endpoints in node-name order; omit incomplete addresses. Retain the original singular connection fields for existing consumers, and synthesize a single endpoint for connection-only metadata, including local deployments.
- Interpret endpoint availability from saved metadata. Information commands perform no network probes. Connection attempts give each candidate a bounded share of the remaining overall deadline, including stalled handshakes. Login rejection ends the attempt without retrying another node.
- Keep the exact legacy host for unmarked deployments. For opted-in deployments, pass eligible hosts through the driver transport and login APIs and preserve TLS fingerprint validation for every attempt. The shared-port limitation is checked explicitly; inconsistent eligible ports produce an actionable error rather than silently dropping nodes.
- Update info text, JSON, and generated connection instructions using the same endpoint model. Clearly distinguish topology from connectable endpoints so external clients can select eligible hosts.

## Risks / Trade-offs

- Unmarked deployments created after the license switch also use the conservative primary-only path. Migration is explicitly deferred.
- Older launchers can still connect through the primary host within the existing deployment-version compatibility rules. The existing version guard can require a launcher upgrade across releases. Test the pre-change binary against new deployment metadata; avoid changing state schemas or compatibility versions.
- External preset authors must declare multi-node support only when their installed database accepts logins on all advertised nodes. Document the metadata contract in the preset guide.
- The driver uses one certificate fingerprint and shared port. Cover fingerprint rejection and port mismatches with focused tests; bundled providers use a shared certificate and port.

## Delivery and Validation

Keep endpoint resolution/reporting, bounded transport attempts, and multi-node selection in separate purposeful commits, each with its own tests and relevant docs. Archive OpenSpec artifacts into a separate final commit after validation. Run `task fmt`, `task all`, focused real-driver transport/TLS tests, cloud-free CLI evidence, and a scoped user evaluation. Product-facing endpoint and initialization evidence is in launcher tests marked `openspec("deployment-info-reporting")` and `openspec("multi-node-connections")`; focused Go integration tests cover real-driver transport/TLS and optional current/preceding launcher binaries. Cloud provisioning is not needed for the controlled transport tests; report any live-cluster validation separately.
