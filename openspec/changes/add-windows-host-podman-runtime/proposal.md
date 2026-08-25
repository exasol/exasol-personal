## Why

Local deployments are unavailable on Windows, so Windows users are pushed to a cloud deployment to try Exasol. Windows can run the same Podman installation policy Linux already uses, but its prerequisites differ: Podman is not usually present, installing it changes state shared beyond the deployment, and the containers run inside a WSL2 machine rather than on the host kernel.

Satisfying those prerequisites also exposes a gap that is not Windows-specific. Preparation previously ran after the deployment recorded an operation in progress, and it decided for itself whether to prompt. A declined or failed prerequisite therefore stranded the deployment mid-operation, and a non-interactive run silently approved host changes because a non-terminal stdin was treated as consent.

## What Changes

- Support the standard `local` deployment on Windows AMD64 through host Podman and its default Podman machine.
- Generalize the Linux host runtime into one direct-host runtime whose platform prerequisites, start-time re-checks, and command execution environment are injected per platform.
- Move host-change approval to the command layer: runtimes declare the commands they intend to run, and the CLI decides how to ask. Add `--auto-approve` to `deploy`, `install`, and `start`.
- Run host preparation before the deployment records an operation in progress, so declined or failed prerequisites leave the deployment retryable.
- Publish the database port on the IPv4 loopback explicitly, so WSL2's localhost relay cannot expose an unreachable IPv6-only endpoint.
- Flush Nano's startup writes inside the Podman machine on Windows, where the page cache holding them belongs to the machine's kernel rather than the host.

## Capabilities

### New Capabilities

- `windows-host-runtime-environment`: Make a Windows host ready to run local deployments by ensuring Podman is available, installing it through Windows Package Manager only with approval, and ensuring its default machine is running.

### Modified Capabilities

- `exasol-local-deployment`: Support the standard local deployment on Windows AMD64, including platform-specific configuration, unattended approval, status, health, and shell behavior.
- `local-runtime-boundaries`: Select Windows as a supported local platform, keep host-change approval and progress presentation in the command layer, and prepare the host before recording an operation in progress.

## Impact

The change affects local runtime selection and the direct-host runtime, the Windows environment preparer, the Podman published-port binding, the deployment and start workflows, the local and cloud backend interfaces, the `deploy`, `install`, and `start` commands, reachability and local diagnostics, unit and integration tests, Windows CI diagnostics, README prerequisites, and user-visible release notes.

Two user-visible behaviors change beyond Windows support. Host preparation in a non-interactive run now fails instead of proceeding without approval, which `--auto-approve` restores for unattended use. Published database ports bind the IPv4 loopback rather than the wildcard address on every platform. macOS VM behavior and Linux host behavior are otherwise unchanged, and Windows ARM64 remains unsupported.
