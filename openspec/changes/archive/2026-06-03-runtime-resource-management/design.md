## Context

The current tofu integration embeds platform-specific binaries into Go source files and writes them into the deployment directory as part of initialization. That approach keeps execution simple, but it makes the launcher know too much about tofu and ties runtime behavior to compile-time asset generation.

This change introduces a generic runtime resource manager. The launcher will consume a resource definition file and use the named resources it defines. The resource manager will resolve those requests against embedded metadata, a deployment-local cache, and the current platform.

## Goals / Non-Goals

**Goals:**
- Keep tofu-specific details out of launcher flow code.
- Make resource selection declarative and data-driven.
- Reuse downloaded artifacts from a deployment-local cache when they are still valid.
- Refresh cached artifacts when the embedded resource spec changes.
- Make platform-dependent selection testable without patching runtime globals.

**Non-Goals:**
- Redesigning deployment-directory compatibility.
- Adding a shared cache outside the deployment directory.
- Solving generalized package management for arbitrary third-party software.

## Decisions

### Introduce a resource manager library

The new library owns the resource directory inside the deployment directory, the `resources.json` cache index, and the full resource lifecycle:

1. resolve the requested resource from the embedded spec
2. check the deployment-local cache
3. download the artifact if it is missing or stale
4. verify the downloaded artifact checksum
5. optionally extract it
6. return the local path to the caller

Rationale:
- The launcher stays small and generic.
- The cache logic becomes unit-testable in isolation.
- Future runtime assets can reuse the same mechanism.

### Use two constructors for platform control

The manager will expose:

- `NewResourceManager(...)` for production, which defaults to `runtime.GOOS` and `runtime.GOARCH`
- `NewResourceManagerForPlatform(...)` for tests and explicit control

Rationale:
- Production call sites stay simple.
- Tests can exercise platform resolution deterministically.
- The manager remains immutable after construction.

### Keep a single embedded `resources.yaml`

The binary will embed one resource definition file. The manager parses it into a `ResourceSpec` at startup.

Rationale:
- A single file is more developer-friendly than splitting resource metadata across multiple files.
- The resource manager can still resolve platform-specific artifacts internally.

### Make each resource define platform-specific artifacts directly

Each resource entry will have:

- `extract: bool`
- `artifact:`
  - one or more platform-specific entries

Each artifact entry keeps `url` and `sha256` adjacent, and may also include:

- `download_path`, the implementation-specific path used when the URL does not already end in a filename
- `resource_path`, the implementation-specific path inside the archive that is returned to the caller when `extract: true`

Rationale:
- The YAML stays easy to scan and update.
- The schema avoids ambiguous fallback behavior and removes the need for shared or common artifacts.
- Checksum maintenance stays close to the download URL.

### Treat URL or checksum changes as the staleness signal

The resource manager will not maintain an explicit resource version. Instead, `resources.json` records the source URL and checksum used for each cached resource. If either the resolved URL or the checksum changes, the cached artifact is stale and must be refreshed.

Rationale:
- The cache stays simple.
- Resource invalidation follows the source of truth.
- Developers can refresh artifacts by changing the URL or checksum in `resources.yaml`.

### Scope cached artifacts by resource ID

The resource manager will store artifacts under `<deployment-dir>/resources/<resource-id>/<artifact>`.

Rationale:
- Each logical resource gets an isolated cache namespace.
- The path layout is easy to inspect and reason about.
- Resource-specific extraction or replacement can happen without cross-resource collisions.

### Use per-platform download paths only when they are needed

The resource manager will use the URL path basename as the default download path. The platform-specific `download_path` overrides that default under the resource directory for that logical resource.

Rationale:
- Most artifact URLs already name the file they point to.
- The explicit path is only needed for URLs that end at a download endpoint rather than a file.
- The explicit path is an override, not a separate mode.
- The cache layout stays predictable for resources with or without explicit paths.
- Path containment is checked against the resource directory rather than inferred from path segments alone.

### Verify the downloaded artifact after download, not the extracted file

Checksum validation applies only to freshly downloaded artifacts, before they are renamed into the cache location. If `extract: true`, extraction happens after checksum verification.

Rationale:
- The checksum matches the upstream distribution artifact.
- Developers can update the expected hash directly in `resources.yaml`.
- Cache hits are validated by metadata and file presence; the download path is where integrity is enforced.
- The manager does not need to invent additional hashes for extracted outputs.

### Derive extraction output paths from the archive name

When extraction is enabled, the manager will derive the extraction directory from the archive filename by stripping the archive extension, and return the requested `resource_path` when one is configured.

Rationale:
- No extra `output_name` field is needed.
- The archive name already identifies the extracted directory.
- A configured archive entry path keeps the caller-facing result stable even when the archive contains multiple files.

### Support only `.tar.gz` and `.tgz` extraction

The manager will only extract resources whose downloaded artifacts end in `.tar.gz` or `.tgz`.

Rationale:
- The supported archive formats are explicit and narrow.
- The implementation can keep a single extraction path for both supported extensions.

### Keep the implementation single-threaded for now

The initial version will not add concurrency controls for simultaneous requests.

Rationale:
- The current use case is small and developer-managed.
- The API stays easier to test and reason about.
- Parallelization can be added later inside the manager without changing the launcher-facing API.

## Risks / Trade-offs

- [Checksum mismatches can be caused by upstream drift or local corruption] -> Report expected and actual hashes clearly so developers can fix the YAML or delete the bad cache entry.
- [Archive naming rules can be subtle for compound extensions] -> Derive extracted names from the full archive filename, not only the last extension.
- [The manager could become too generic] -> Keep the first version narrowly focused on deployment-local runtime artifacts and the launcher's current needs.
