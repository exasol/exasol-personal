## ADDED Requirements

### Requirement: Exasol Personal deploys to IONOS Cloud with token authentication
`exasol install ionos` SHALL provision an Exasol Personal cluster in the IONOS Cloud contract that the API token in `IONOS_TOKEN` belongs to, using the `ubuntu` installation preset by default.

#### Scenario: Install a single-node cluster with defaults
- **WHEN** a user with a valid token in `IONOS_TOKEN` runs `exasol install ionos`
- **THEN** a single-node cluster with 4 dedicated cores and 16 GB RAM is provisioned in `de/fra`
- **AND** `exasol connect -c "SELECT 1"` returns a result

#### Scenario: Install a multi-node cluster
- **WHEN** a user runs `exasol install ionos --cluster-size 2`
- **THEN** a two-node cluster is provisioned and each node has its own public IP address

#### Scenario: Install without a valid token
- **WHEN** a user runs `exasol install ionos` without a valid token in `IONOS_TOKEN`
- **THEN** the installation fails with an authentication error

### Requirement: IONOS node size is selected as cores and RAM
`exasol install ionos` SHALL accept `--instance-type` as `<cores>x<ram_gb>` for the dedicated core servers of every node, together with `--location`, `--cpu-family`, `--disk-type`, `--os-volume-size`, and `--data-volume-size`.

#### Scenario: Install with a custom size
- **WHEN** a user runs `exasol install ionos --instance-type 8x32 --data-volume-size 200`
- **THEN** each node has 8 dedicated cores, 32 GB RAM, and a 200 GB data volume

#### Scenario: Reject a malformed instance type
- **WHEN** a user runs `exasol install ionos --instance-type large`
- **THEN** the installation fails with an error that names the `<cores>x<ram_gb>` format
- **AND** no IONOS Cloud resources are created

#### Scenario: Reject a size the location does not offer
- **WHEN** the requested cores or RAM exceed the per-server limits of the selected location and CPU family
- **THEN** the installation fails with an error that names those limits
- **AND** no servers are created

### Requirement: IONOS deployments keep addresses and data across stop and start
`exasol stop` SHALL shut off every server of an IONOS deployment, and `exasol start` SHALL power them on again with the same public IP addresses and database contents.

#### Scenario: Stop and start a deployment
- **WHEN** a user runs `exasol stop` and then `exasol start` on an IONOS deployment
- **THEN** `exasol info` reports the same node IP addresses as before the stop
- **AND** data written before the stop can be queried after the start

### Requirement: Destroying an IONOS deployment removes the resources it created
`exasol destroy` SHALL remove every IONOS Cloud resource that the deployment created, including its data center and reserved IPs, and SHALL leave a data center selected with `--datacenter-id` in place.

#### Scenario: Destroy a deployment with its own data center
- **WHEN** a user runs `exasol destroy` on a deployment installed without `--datacenter-id`
- **THEN** the deployment's data center and reserved IPs no longer exist in the contract

#### Scenario: Destroy a deployment in an existing data center
- **WHEN** a user runs `exasol install ionos --datacenter-id <id>` and later `exasol destroy`
- **THEN** the deployment's servers, volumes, LAN, and reserved IPs no longer exist
- **AND** the data center with that ID still exists
