## MODIFIED Requirements

### Requirement: Container port mappings produce loopback endpoints

For local TCP ports with a positive `hostPort`, the launcher SHALL publish
`containerPort` on the requested loopback `hostIP` and host port, defaulting
an omitted host IP to `127.0.0.1`. A declaration containing only
`containerPort` SHALL provide an internal service port. Status SHALL report
the effective published host IP and port. When a published port declares an
application protocol, status SHALL additionally report a complete URL built
from that scheme, the effective published host address, and any declared path.
Non-loopback publication requests SHALL receive an explicit local-support
validation error.

#### Scenario: Published endpoint is reachable from the local host

- **WHEN** a sidecar with a loopback host mapping is enabled
- **THEN** the service is reachable at the IP and port reported by status
- **AND** the host listener is bound exclusively to the requested loopback IP

#### Scenario: Published endpoint reports its complete URL

- **WHEN** a running sidecar publishes a port that declares an application
  protocol
- **THEN** text and JSON status report a URL carrying that scheme, the
  effective published host address, and the declared path
- **AND** the service answers at that URL

#### Scenario: Published endpoint without a declaration reports its address

- **WHEN** a running sidecar publishes a port that declares no application
  protocol
- **THEN** status reports that endpoint's host IP and port and no URL

#### Scenario: Internal service port stays internal

- **WHEN** a sidecar declares a container port with host publication omitted
- **THEN** its service is reachable by deployment DNS and status reports an
  empty published endpoint list
- **AND** an application protocol declared for that port leaves that list empty

#### Scenario: Broad publication request receives a validation error

- **WHEN** a saved sidecar requests a non-loopback host IP
- **THEN** validation identifies that mapping and the supported loopback scope
