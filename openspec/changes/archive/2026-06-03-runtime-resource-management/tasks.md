## 1. Resource manager schema

- [x] 1.1 Update the embedded `resources.yaml` schema to use platform-specific artifacts only.
- [x] 1.2 Add per-platform download paths for artifacts whose URLs do not end in a filename.
- [x] 1.3 Add per-platform archive entry paths that are returned to the caller when extraction is enabled.
- [x] 1.4 Reject archive-entry paths when extraction is not requested.

## 2. Cache and resolution behavior

- [x] 2.1 Keep cache metadata in `resources.json` for source URLs and checksums.
- [x] 2.2 Preserve stale-cache behavior when a resource URL changes.
- [x] 2.3 Scope cached artifacts by `<deployment-dir>/resources/<resource-id>/<artifact>`.

## 3. Download and extraction behavior

- [x] 3.1 Implement artifact download with URL basename fallback and explicit download path overrides.
- [x] 3.2 Implement `sha256` verification for downloaded artifacts.
- [x] 3.3 Support optional extraction for `.tar.gz` and `.tgz` archives only.
- [x] 3.4 Return the extracted archive entry path when a resource path is configured.
- [x] 3.5 Report checksum mismatches with expected and actual hashes.

## 4. Launcher integration

- [x] 4.1 Update the tofu resource definition to the revised schema.
- [x] 4.2 Keep tofu path resolution pointed at the resource-managed artifact path.
- [x] 4.3 Remove obsolete build-time tofu download wiring after the runtime flow is in place.

## 5. Validation and docs

- [x] 5.1 Add unit tests for platform resolution, cache hits/misses, checksum mismatch handling, path validation, and archive extraction.
- [x] 5.2 Update development documentation to describe the revised runtime resource workflow.
