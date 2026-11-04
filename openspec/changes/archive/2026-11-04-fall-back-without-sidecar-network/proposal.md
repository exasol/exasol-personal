## Why

Since v2.4.0-rc1, every local deployment on Podman starts the database on a
deployment network so that sidecars can join without a restart. On kernels
without the nftables support that Podman's network needs, such as the one
shipped with WSL 2.6, creating the database on that network fails, so
`exasol install local` and `exasol start` fail, which v2.3.0 did not. The Linux
launcher inside WSL and the Windows launcher are both affected.

## What Changes

- When the database cannot be started on the deployment network because the
  kernel lacks nftables support, the launcher starts it without the network,
  so local deployments without sidecars work on WSL 2.6 again.
- Enabling a sidecar on such a deployment fails without saving it, with
  guidance: on Windows or
  inside WSL, that sidecars on Podman require WSL 2.7 or later and to run
  `wsl --update`, then `exasol stop` and `exasol start`; on other Linux hosts,
  that the kernel lacks nftables support, with a link to troubleshooting.
- Starting such a deployment with an enabled sidecar brings the database up and
  reports each enabled sidecar as failed with the same guidance, as start does
  for any sidecar failure; status reports it too.
- Once a later start succeeds on the network, for example after `wsl --update`
  or switching Podman to iptables, sidecars can be enabled again.
- Documentation states that sidecars on Podman require WSL 2.7 or later and
  that a restart is needed after either fix.

## Capabilities

### Modified Capabilities

- `sidecar-networking`: new databases use the deployment network when the host
  supports it; hosts without nftables support keep the database running
  without it and refuse sidecars with guidance.

## Impact

- The local start path, sidecar enable and status, and the recovery guidance
  in the CLI.
- Go unit tests for the fallback and the guidance; a manual check on WSL 2.6 and
  WSL 2.7 or later with the Linux and Windows launchers.
- User documentation (troubleshooting, system requirements) and the changelog
  entry for sidecars.
