## Why

Local mode now depends on the real macOS `Virtualization.framework` path and on a build-time embedded payload bundle. The current release pipeline still treats macOS artifacts like generic cross-compiled launcher binaries, which is no longer sufficient for a valid local-mode launcher build.

We need a release path that builds the macOS arm64 launcher on macOS, embeds the pinned local runtime payload during that build, signs the launcher with the virtualization entitlement, notarizes the shipped artifact, and publishes it as part of the normal release flow.

## What Changes

- Build the `darwin/arm64` launcher on a macOS CI runner instead of Linux cross-compilation.
- Generate the embedded local runtime payload bundle during CI from pinned build inputs.
- Sign the macOS launcher with the required virtualization entitlement and notarize the shipped macOS artifact.
- Publish the macOS launcher through the existing release flow alongside the non-macOS artifacts.
- Add CI validation so macOS release failures surface before a tagged release is declared complete.

## Impact

- Changes GitHub Actions release workflow structure.
- Changes how the macOS launcher artifact is built and attached to releases.
- Keeps the local runtime product contract unchanged; this change is delivery and release engineering work.
