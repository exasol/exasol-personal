## ADDED Requirements

### Requirement: Connection information reports public database endpoints

`exasol info`, `exasol info --json`, and connection instructions SHALL report the public database endpoints recorded in deployment metadata, including each node identity, host, database port, and connection eligibility. JSON endpoint objects SHALL name the permission flag `eligible`, reflecting the combination of saved installation login permission and infrastructure-reported public exposure. Endpoint information SHALL use stable node order. Existing singular connection fields SHALL retain their values.

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

#### Scenario: Eligible endpoint is currently unavailable

- **WHEN** a user requests connection information for a node permitted by saved deployment configuration that is currently unreachable
- **THEN** its JSON endpoint reports `eligible: true` and the text identifies eligibility as recorded configuration

#### Scenario: Infrastructure exposes a subset of nodes

- **WHEN** a deployment permits logins on all nodes and infrastructure reports public database endpoints for only some nodes
- **THEN** connection information lists those public endpoints in node order and graphical-client instructions use an eligible host and its reported port

#### Scenario: Known nodes have no public endpoints

- **WHEN** a deployment permits logins on all nodes and infrastructure reports nodes without public database endpoints
- **THEN** JSON reports `connection.endpoints` as an empty array and text explains that infrastructure has recorded no public database endpoints
