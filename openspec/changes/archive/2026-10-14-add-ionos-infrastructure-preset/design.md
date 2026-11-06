## Context

The preset was first built and validated as an external preset against a live IONOS Cloud contract with the `ionos-cloud/ionoscloud` OpenTofu provider. It follows the STACKIT preset's structure and reuses the `ubuntu` installation preset unchanged.

## Goals / Non-Goals

**Goals:**

- Same lifecycle and artifacts as the other cloud presets: `deployment.json`, `secrets.json`, `node_access.pem`, and the host contract in `/etc/exasol_launcher`.
- Token-only authentication with no credentials in preset files or generated artifacts.

**Non-Goals:**

- CUBE and vCPU servers, managed Kubernetes, DBaaS, NAT gateways, and load balancers.
- Username and password authentication.

## Decisions

- **Dedicated core servers sized as `<cores>x<ram_gb>`.** IONOS has no named flavors for dedicated core servers. The preset parses `instance_type`, rejects malformed or undersized values before creating resources, and checks cores and RAM against the location's limits for the selected CPU family before creating servers. Contract limits are enforced by IONOS.
- **File overlays are delivered inline in cloud-init.** The installation and infrastructure file overlays are gzip-compressed into user data instead of being uploaded to object storage. The compressed payload is about 10 KB and was accepted by IONOS, so the preset avoids a separate object storage service and its credentials. The `bootstrap-asset-distribution` capability continues to cover only the providers it names. If overlays grow past what IONOS accepts, object storage can be added the way it was for STACKIT.
- **Cloud-init creates the `ubuntu` user.** IONOS Ubuntu images only provision `root`, while the installation preset runs as `ubuntu`. The SSH key passed to IONOS is also required by IONOS for public images.
- **One public LAN with reserved IPs.** Each node's NIC carries its reserved public IP, which survives `SHUTOFF`. Cluster traffic is allowed between node IPs by firewall rules.
- **Firewall rules are separate resources.** The provider does not read back inline NIC firewall rules, so inline rules would be recreated on every later apply. IONOS rules match a single source IPv4, so `allowed_cidr` accepts `0.0.0.0/0` or one `/32` address.
- **The data volume is matched as `vdb`.** It is attached after the server exists, so its ID cannot be part of the server's user data.
- **One data center per deployment by default.** `exasol destroy` removes it. `--datacenter-id` reads an existing data center instead, uses its location, and leaves it in place on destroy.
- **Labels on servers only.** The provider supports labels on servers only, and IONOS label values accept only `[a-z0-9]`, so values are lowercased and stripped. Other resources carry the deployment ID in their names.
- **Stop and start map to `vm_state`.** `power_state` selects `RUNNING` or `SHUTOFF`.

## Risks / Trade-offs

- [The multi-node layout has not run live yet, because the test contract allows one reservable IP] → Verify with `--cluster-size 2` on a contract with at least two reservable IPs before release.
- [No automated live coverage, because CI has no IONOS credentials] → The acceptance flow is run manually against IONOS Cloud with the built launcher.
- [Resources from interrupted deployments are not found by `tools/cleanup`] → Users follow the existing troubleshooting guidance; an IONOS collector is a follow-up.
