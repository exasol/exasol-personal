## 1. Release workflow split

- [x] 1.1 Update the release workflow so non-macOS artifacts continue through the existing Linux-hosted path.
- [x] 1.2 Add a macOS-hosted release job for the `darwin/arm64` launcher.
- [x] 1.3 Ensure the macOS artifact is attached to the same release as the Linux-hosted artifacts.

## 2. Embedded payload packaging

- [x] 2.1 Add CI steps that generate the embedded local runtime bundle from pinned `.run`, kernel, and initrd inputs.
- [x] 2.2 Fail the build if any payload input or generated metadata is missing or invalid.
- [x] 2.3 Ensure the macOS launcher build consumes the generated embedded payload bundle instead of placeholder assets.

## 3. Signing and notarization

- [x] 3.1 Sign the macOS launcher with the virtualization entitlement in CI.
- [x] 3.2 Notarize the shipped macOS artifact in CI.
- [x] 3.3 Verify the signed launcher exposes the expected entitlement before release publication.

## 4. Validation and documentation

- [x] 4.1 Add CI checks that fail the release when the macOS build, signing, or notarization steps fail.
- [x] 4.2 Update release and CI documentation to describe the macOS runner build path and required inputs/secrets.
- [x] 4.3 Add or update tests and workflow checks for the embedded payload packaging path where practical.
