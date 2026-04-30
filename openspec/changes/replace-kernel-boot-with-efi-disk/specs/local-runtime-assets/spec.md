# local-runtime-assets Specification (Delta)

## MODIFIED Requirements

### Requirement: Versioned local runtime payload distribution

The system SHALL obtain the local runtime payload as a pair of versioned
artifacts from a product-owned HTTP location: an EFI disk image (the generic
Alpine VM) and the Exasol installer binary the launcher delivers to the guest.

#### Scenario: Resolve payload metadata for local deployment

- GIVEN the user is preparing a local deployment
- WHEN the launcher resolves the required payload
- THEN it uses product-owned payload metadata that identifies, for the selected
  guest architecture, both a versioned EFI disk image and a versioned
  installer binary
- AND the metadata describes each artifact as a single asset entry
  (`disk` and `run`) rather than a kernel and initrd pair

### Requirement: Payload verification and caching

The system SHALL verify and cache both downloaded local runtime artifacts (the
EFI disk image and the installer binary) before using them.

#### Scenario: Download and verify artifacts

- GIVEN one or both required artifacts are not present in the local cache
- WHEN the launcher downloads the missing artifact(s)
- THEN it verifies each one against expected integrity metadata
- AND stores the verified artifact in an Exasol-owned cache location

#### Scenario: Reuse cached artifacts

- GIVEN both required artifacts are already present in the local cache
- WHEN the launcher prepares the local runtime
- THEN it reuses the cached artifacts instead of downloading them again

#### Scenario: Reject invalid artifact

- GIVEN a downloaded artifact fails integrity verification
- WHEN the launcher validates it
- THEN it refuses to use the artifact
- AND reports a clear verification error

#### Scenario: Extract archived disk image

- GIVEN the disk asset filename indicates a `.tar.xz` archive
- WHEN the launcher caches the asset
- THEN after sha256 verification it extracts the archive
- AND selects the first `.img` entry inside as the cached disk source for
  subsequent staging

### Requirement: Deployment records selected payload identity

The system SHALL persist the selected payload identity into deployment-owned
local runtime state.

#### Scenario: Persist payload version, architecture, and artifact paths

- GIVEN the launcher has selected a payload for a local deployment
- WHEN it writes deployment-owned local runtime state
- THEN that state records the payload version
- AND records the selected guest architecture
- AND records the cached EFI disk image path
- AND records the cached installer binary path

### Requirement: Launcher-owned guest execution

The system SHALL retain ownership of guest execution: the launcher is
responsible for delivering the Exasol installer binary into the booted guest
and for invoking it.

#### Scenario: Launcher delivers and invokes the installer

- GIVEN the launcher has prepared a local runtime
- WHEN the VM boots
- THEN the installer binary is present at the launcher-controlled per-deployment
  path inside the guest
- AND a launcher-authored start script invokes the installer inside the guest
  on boot

## ADDED Requirements

### Requirement: Per-deployment writable disk

The system SHALL stage a per-deployment writable copy of the cached disk image
before booting the local VM, so concurrent deployments do not share a writable
disk.

#### Scenario: First deploy stages the disk

- GIVEN a deployment directory with no staged disk image
- WHEN the launcher prepares the local runtime
- THEN it copies the cached disk image into a per-deployment path inside the
  deployment directory
- AND uses that copy as the VM boot disk

#### Scenario: Second deploy reuses the staged disk

- GIVEN a deployment directory with a staged disk image whose recorded payload
  identity matches the current selection
- WHEN the launcher prepares the local runtime
- THEN it reuses the staged disk image without copying again

#### Scenario: Payload change re-stages the disk

- GIVEN a deployment directory with a staged disk image whose recorded payload
  identity differs from the current selection
- WHEN the launcher prepares the local runtime
- THEN it replaces the staged disk image with a fresh copy of the new cached
  source

### Requirement: Per-deployment payload share

The system SHALL stage the launcher-owned guest payload (the Exasol installer
binary and a launcher-authored start script) into a per-deployment payload
share directory before booting the local VM. The directory is exposed to the
guest as a single virtio-fs share at the contract path `/mnt/host`.

#### Scenario: First deploy stages the payload share

- GIVEN a deployment directory with no staged payload share contents
- WHEN the launcher prepares the local runtime
- THEN it creates a per-deployment payload share directory
- AND copies the cached installer binary into that directory under the contract
  filename
- AND writes a launcher-authored start script into that directory under the
  contract filename
- AND configures the VM with a single virtio-fs share whose source is that
  directory and whose guest mount is `/mnt/host`

#### Scenario: Second deploy reuses the staged installer

- GIVEN a deployment directory with a staged installer whose checksum matches
  the cached source
- WHEN the launcher prepares the local runtime
- THEN it reuses the staged installer without copying again

#### Scenario: Installer change re-stages the payload share

- GIVEN a deployment directory with a staged installer whose checksum differs
  from the cached source
- WHEN the launcher prepares the local runtime
- THEN it replaces the staged installer with a fresh copy of the new cached
  source

#### Scenario: Start script is rewritten on every deploy

- GIVEN a deployment directory with an existing staged start script
- WHEN the launcher prepares the local runtime
- THEN it rewrites the start script from the launcher binary's embedded copy
  so that it always reflects the current launcher version's contract
