## Why

Local deployments already run the sidecars they enable. Cloud deployments save
the same definitions but have nowhere to run them, so the services a user
configures stop at the edge of their own machine. Giving cloud nodes a sidecar
runtime applies one saved definition to every node.

Related issue: [SPOT-32683](https://exasol.atlassian.net/browse/SPOT-32683).

## What Changes

- Discover one sidecar host per cloud node and apply the deployment's saved
  definitions to each of them through the existing SSH transport.
- Run each cloud sidecar as a service of the node's database service, so the
  node starts, stops, and restarts it with the database.
- Deliver resolved connection values as container secrets created over private
  standard input.
- Use host networking on every node, with `database` resolving to the node's
  private database address.
- Admit declared host ports through the provider firewall for the deployment's
  allowed address range, following enablement across stop and start.

## Capabilities

### Modified Capabilities

- `sidecar-lifecycle`: Cloud hosts sharing one desired definition, and
  sidecars following their node's database service.
- `sidecar-networking`: Host networking and firewall admission for declared
  cloud endpoints.

## Impact

The change affects cloud host discovery, Podman execution through the SSH
transport, the infrastructure presets' ingress rules, and the launcher-managed
infrastructure variables.

User and contributor documentation and the unreleased changelog describe cloud
sidecars. Cloud evidence uses the existing reusable live deployment.

A declared `hostPort` becomes reachable from the deployment's allowed address
range, so the user guide records that choosing one is an exposure decision.
