# IONOS Cloud infrastructure preset

Provisions Exasol Personal on IONOS Cloud dedicated core servers with Ubuntu 24.04, for use with the `ubuntu` installation preset.

## Prerequisites

- An IONOS Cloud contract whose user may create datacenters and reserve IP blocks.
- An API token exported as `IONOS_TOKEN`. The provider reads it from the environment only.

## Resources per deployment

- One datacenter in `location` (default `de/fra`), or the existing datacenter given by `datacenter_id`, with one public LAN. An existing datacenter is never deleted by `exasol destroy`.
- One reserved IP block with one IP per node. Node addresses survive stop and start.
- One dedicated core server per node, named after the node (`n11`, `n12`, …), with an OS volume and a separate data volume.
- Firewall rules on each NIC allowing ports 22, 2581, 8443, 8563, 20002 and 20003 from `allowed_cidr`, plus all traffic between cluster nodes.

## Configuration

- `datacenter_id` deploys into an existing datacenter instead of creating one, for example `--datacenter-id <uuid>`. Its location then overrides `location`.
- `instance_type` is `<cores>x<ram_gb>` (default `4x16`). It must fit the location's per-server limits for the selected `cpu_family` (default `AMD_EPYC`).
- `disk_type` applies to both volumes (default `SSD Premium`). `os_volume_size` and `data_volume_size` default to 100 GB.
- `allowed_cidr` accepts `0.0.0.0/0` or a single address as `<ip>/32`, because IONOS firewall rules match a single source IP.

## Examples

```bash
exasol install ionos
exasol install ionos --instance-type 8x32 --data-volume-size 200
exasol install ionos --cluster-size 2 --instance-type 8x32 --data-volume-size 200
exasol install ionos --location de/txl --cpu-family INTEL_ICELAKE --disk-type "SSD Standard"
exasol install ionos --datacenter-id <uuid>
```

The contract must cover cores and RAM per server, cores and RAM in total (`cluster_size` × `instance_type`), and one reservable IP per node.

## Costs

Reserved IPs and volumes are billed while servers are stopped.

## Limitations

- Only dedicated core (`ENTERPRISE`) servers are supported. CUBE and vCPU servers are not.
- Labels are applied to servers only.
