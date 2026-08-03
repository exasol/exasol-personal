## Why

External preset sources (git repositories and archives) currently must expose the preset
directory at the resolved root: `verifyPresetManifest` checks the resolved path for
`infrastructure.yaml` / `installation.yaml`, and the CLI builds a `ResourceDefinition` with
`URL` only, so the runtime-artifact `ResourcePath` selector is never populated. That forces
every git-hosted preset into its own repository and every archive to root-level presets — a
poor fit for monorepos of related infrastructure/installation presets.

We enable a `#subdir` fragment on external preset URIs so users can select a preset from a
subdirectory of a git clone or archive. This mirrors the well-established Web/Go/pip/npm
fragment idiom and composes cleanly with the existing `@ref` grammar
(`repo.git@v1#infra/aws`).

## What Changes

- CLI preset arguments accept an optional `#<subpath>` fragment after the URL (and after any
  `@<ref>` suffix for git URLs). The resolved preset directory is the given subdirectory of
  the cloned repository or extracted archive.
- The runtime-artifact layer allows `ResourcePath` on git sources (currently rejected because
  git sources use `Extract: false`) and applies it when resolving the entry.
- `deploy.ResolvePreset` parses the fragment off the URI before building the
  `ResourceDefinition` and populates `ArtifactSpec.ResourcePath`.
- `file://` directory URIs continue to reject the fragment (users can point at the
  subdirectory directly). All other supported schemes accept it.

## Impact

- `internal/runtimeartifacts/spec.go`: loosen `ResourcePath` validation for git sources.
- `internal/runtimeartifacts/manager.go`: apply `ResourcePath` in the non-extract branch when
  the URL is a git source.
- `internal/runtimeartifacts/preset_uri.go` (new): `ParsePresetURI` helper that splits the
  fragment off a preset URI.
- `internal/deploy/preset_external.go`: wire the parsed subpath into `ResourceDefinition`.
- Cache identity: unchanged — `ResourcePath` is already part of `artifactIdentity`, so
  different subpaths produce distinct cache entries (accepted duplication; shallow clones).
- Backward compatible: no existing external preset URI contains `#`.

## Capabilities

### Modified Capabilities

- `external-preset-resolution`: adds the subdirectory fragment syntax and its precedence with
  `@ref`.
- `preset-source-fetching`: adds the subdirectory selection on git and archive sources.
