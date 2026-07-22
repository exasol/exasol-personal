## ADDED Requirements

### Requirement: An embedded resource source materializes resources from data compiled into the binary
For resources marked `embed: true`, the resource manager SHALL resolve exclusively from embedded data compiled into the binary, never from network-based sources. For resources not marked `embed: true`, the resource manager SHALL resolve exclusively from its network-based sources, exactly as before this capability existed.

#### Scenario: Embedded data resolves a resource without network access
- **WHEN** a resource is marked `embed: true` and matching data is present in the binary's embedded registry
- **THEN** the resource manager materializes the resource from that embedded data without contacting any network source

#### Scenario: Missing embedded data is a hard failure, not a fallback
- **WHEN** a resource is marked `embed: true` but no matching data is present in the binary's embedded registry
- **THEN** the resource manager fails to resolve that resource, and does not attempt to resolve it from any network-based source

#### Scenario: A resource without embed: true never consults embedded data
- **WHEN** a resource is not marked `embed: true`
- **THEN** the resource manager resolves it only from its network-based sources, regardless of whether the binary's embedded registry contains data under the same resource identifier

#### Scenario: Embedded resource extraction reuses existing extraction
- **WHEN** an `embed: true` resource with archive extraction enabled is materialized from embedded data
- **THEN** the resource manager extracts it using the same extraction mechanism used for a network-fetched archive of that format

#### Scenario: Embedded resource data is not re-verified against a checksum at resolution time
- **WHEN** a resource is materialized from embedded data
- **THEN** the resource manager does not re-verify that data against the resource's configured checksum
