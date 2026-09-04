# System requirements

## Launcher platforms

The Exasol Launcher runs on:

- macOS 12 Monterey or later;
- Linux on AMD64 or ARM64; and
- Windows 10 or later on AMD64.

The launcher platform and the database deployment target are separate. A launcher running on any
supported platform can manage a cloud deployment.

## Local database

A local database always runs as a single node. Platform-specific requirements are:

- **macOS:** macOS 15 Sequoia or later on Apple Silicon with at least 8 GB of host memory. The
  launcher supplies Podman in a managed virtual machine.
- **Linux:** AMD64 or ARM64 with Podman installed and available in `PATH`. The database uses host
  resources directly.
- **Windows:** Windows 10 or later on AMD64, Windows Package Manager (`winget`), and the prerequisites
  for a Podman machine. Windows on ARM64 is not supported for local deployments.

## Cloud database

Cloud deployments run on AWS, Azure, Exoscale, or STACKIT infrastructure. The launcher provisions
Ubuntu 24.04 LTS on x86-64 compute instances.

The cloud account must have permission and quota for the selected compute, storage, and networking
resources. See the provider-specific account setup pages for details.
