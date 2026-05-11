## Overview

The release pipeline should treat the macOS local-mode launcher as a platform-native artifact instead of a generic cross-compiled binary. Linux and Windows artifacts can continue through the existing Linux-hosted GoReleaser path, but the macOS arm64 launcher should be built on a macOS runner where `cgo`, Apple frameworks, signing, and notarization are all available.

## Build Strategy

The pipeline should split release work into two slices:

- a Linux-hosted release job that continues to produce the non-macOS artifacts and owns the canonical release metadata
- a macOS-hosted job that builds the `darwin/arm64` launcher, embeds the pinned local runtime payload, signs it, notarizes the shipped artifact, and uploads that artifact into the same release

This keeps the existing release shape mostly intact while moving only the platform-native launcher work onto macOS.

## Embedded Payload Packaging

The macOS job should generate the embedded payload bundle as part of the build, not rely on a precommitted production bundle. The build inputs should be explicit and pinned for the release:

- Linux ExaNano `.run`
- guest kernel
- guest initrd
- local runtime payload version

The build should fail if any required payload input is missing or if the generated embedded bundle does not match the expected metadata shape.

## Signing And Notarization

The macOS launcher must be signed with the virtualization entitlement and shipped as a notarized macOS artifact. The pipeline should treat signing and notarization as part of the macOS build slice rather than as a separate manual post-process.

The design assumption is:

- release secrets remain CI-managed
- the final launcher artifact is the signed and notarized one that gets attached to the release

## Validation

The release pipeline should validate at least:

- the embedded payload bundle was generated
- the built launcher contains the virtualization entitlement after signing
- notarization completed successfully

Runtime validation that actually boots the local VM can stay in a separate validation workflow if needed, but the release pipeline should at minimum prove that the shipped macOS artifact is a valid signed local-mode launcher.
