## Why

The launcher runs local deployments directly through host Podman on Linux, but Windows users are still rejected even though a Windows Nano artifact and a proven Podman-for-Windows setup flow already exist. Windows needs the same local deployment lifecycle with explicit, safe handling of its Podman and Podman-machine prerequisites.

## What Changes

- Generalize the Linux direct-Podman runtime into a host runtime whose platform environment preparation is injected while keeping the shared container lifecycle unchanged.
- Support the standard `local` deployment on Windows AMD64 through host `podman.exe` and deployment-owned persistent storage.
- Detect or install Podman through winget, refresh the launcher process PATH after installation, and ensure the default Podman machine is running rootful.
- Require interactive approval, or `--auto-approve`, before installing Podman or converting an existing rootless machine.
- Add Windows local lifecycle coverage to the regular integration suite and document the supported platform and prerequisites.

## Capabilities

### New Capabilities

- `windows-host-runtime-environment`: Prepare the Windows host environment for local runtimes, including Podman discovery, winget installation, PATH refresh, and default-machine configuration.

### Modified Capabilities

- `exasol-local-deployment`: Support local deployment lifecycle, configuration, status, health, and documented prerequisites on Windows AMD64.
- `local-runtime-boundaries`: Generalize host runtime preparation and select the Windows host environment without duplicating the common container lifecycle.

## Impact

The change affects local runtime and deployment-backend preparation interfaces, Windows host command execution, CLI approval flags and prompts, platform selection, unit and integration tests, end-user documentation, architecture documentation, and release notes. Linux and macOS behavior remains compatible, Windows ARM64 remains unsupported, and no new Go dependency is required.
