## Why

When a Podman command fails, the launcher reports only the exit status, such as `failed to start Nano container exasol-db-<id>: exit status 126`. Podman's own explanation reaches the terminal only with `--verbose` and never reaches `deployment.log`, so users and support cannot tell why a local deployment failed. On WSL 2.6, for example, the cause was a netavark nftables error that stayed hidden behind the exit status.

## What Changes

- A failed Podman command reports Podman's error line together with the exit status, with or without `--verbose`.
- A failed deployment records the error and Podman's error output in `deployment.log`.
- Sidecar startup errors keep their current wording and redaction, without repeating the cause.
- Docker's error lines are recognised as well, so a Docker runtime that shares the Podman command execution reports its reason the same way.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `linux-host-podman-install`: Podman command failures state Podman's reported reason and record it in the deployment log.

## Impact

- Podman runtime command execution in `internal/localinstall`.
- Deploy failure handling in `internal/deploy`.
- The sidecar startup error helper becomes shared with the Podman runtime.
