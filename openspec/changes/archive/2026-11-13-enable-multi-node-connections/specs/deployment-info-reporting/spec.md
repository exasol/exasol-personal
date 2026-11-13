## ADDED Requirements

### Requirement: Connection information reports public database endpoints

`exasol info`, `exasol info --json`, and connection instructions SHALL report the public database endpoints recorded in deployment metadata, including each node identity, host, database port, and connection eligibility. Endpoint information SHALL use stable node order. Existing singular connection fields SHALL retain their values.

#### Scenario: Eligible multi-node deployment

- **WHEN** a user requests connection information for a deployment supporting connections on all nodes
- **THEN** the output identifies every public database endpoint and shows how to build a multi-host connection

#### Scenario: Legacy multi-node deployment

- **WHEN** a user requests connection information for an unmarked deployment
- **THEN** the output distinguishes the primary connection endpoint from other recorded public node endpoints
- **AND** existing singular host and port fields keep their values

#### Scenario: Local or connection-only metadata

- **WHEN** a running deployment records a singular connection without public node metadata
- **THEN** connection information reports that saved connection as one eligible endpoint
