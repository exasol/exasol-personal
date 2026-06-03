## Why

The launcher currently embeds platform-specific tofu binaries at compile time and writes them into the deployment directory during initialization. That couples builds to binary packaging, makes the launcher aware of tofu details it should not own, and forces resource selection to happen in Go code instead of in a declarative asset spec.

## What Changes

- Add a runtime resource manager library that owns deployment-local resource storage, caching, download, checksum validation, and optional extraction.
- Replace the compile-time tofu binary embedding flow with embedded `resources.yaml` metadata that is parsed into a `ResourceSpec` at runtime.
- Let the launcher request a logical resource name and receive a local filesystem path; the launcher should not need to know how a resource is resolved, cached, or extracted.
- Support one embedded `resources.yaml` file per binary, with platform resolution handled inside the resource manager using runtime `GOOS` and `GOARCH` by default.
- Allow tests to inject an explicit platform into the resource manager through a dedicated constructor.
- Store cache metadata in a deployment-local `resources.json` file so the manager can reuse cached artifacts, remember where each artifact came from, and refresh them when a resource's URL or checksum changes.
- Support platform-specific artifacts for each resource, with optional per-platform download paths for URLs that do not already end in a filename.
- Support per-platform archive entry paths for extracted artifacts so callers can resolve the file inside the extracted archive that should be returned.
- Validate artifact integrity with `sha256` and report expected vs actual checksum on mismatch.
- Optionally extract downloaded `.tar.gz` or `.tgz` archives when a resource entry sets `extract: true`.

## Non-Goals

- Changing deployment-directory compatibility/versioning behavior.
- Introducing a global machine-wide cache.
- Supporting concurrent resource requests in the first version.
- Adding resource name collision resolution beyond the developer-managed resource set in the embedded spec.

## Impact

- Affects tofu handling in the launcher and any future runtime resources that use the same manager.
- Removes the need for compile-time platform-specific tofu asset embedding.
- Changes the developer workflow for updating runtime artifacts and checksums.
