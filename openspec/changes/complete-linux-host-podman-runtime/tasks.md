## 1. Portable Installation Contract

- [x] 1.1 Simplify the Podman start configuration, publish the configured database port to Nano's fixed internal port, and internalize Podman constants
- [x] 1.2 Add runtime-neutral version-check and SLC startup options with shared launcher-state translation and distinct nil versus empty SLC semantics
- [x] 1.3 Extend the shared install interface with exact deployment-container status and focused tests

## 2. Podman Startup Parity

- [x] 2.1 Emit best-effort Podman environment and container diagnostics for materialization, recreation, and startup failures
- [x] 2.2 Configure Nano version-check enablement, identity, operating system, URL, bounded interval, and bounded retry interval
- [ ] 2.3 Reuse, pull, or import configured SLCs, mount available images, and atomically publish their availability status
- [ ] 2.4 Prune only unreferenced exact official-repository and labeled custom-import images for SLC-aware callers

## 3. Data Recovery and Migration

- [ ] 3.1 Recover interrupted initial Nano creation by quarantining partial data and removing only stale uninitialized TLS files
- [ ] 3.2 Migrate legacy overlay-backed `/exa` data through sibling staging without overwriting populated storage or deleting the source prematurely

## 4. Linux Runtime Integration

- [ ] 4.1 Implement Linux runtime status mapping, deployment endpoint recovery, published-port health checks, and Linux-specific reachability diagnostics
- [ ] 4.2 Centralize platform runtime selection for all local workflows, promote Linux host Podman to `local`, remove `local-host`, and make VM sizing and shell behavior platform-specific

## 5. Documentation and Verification

- [ ] 5.1 Document Linux Podman prerequisites, platform resource behavior, the local preset change, and the user-visible compatibility change
- [ ] 5.2 Run strict OpenSpec validation, repository unit tests, lint, and build
- [ ] 5.3 Smoke-test install, status, SQL connectivity, SLC restart persistence, stop/start, and removal with the default Linux `local` deployment
