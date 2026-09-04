# System requirements

## Launcher platforms

The Exasol Launcher in release 2.2 runs on:

- macOS 12 Monterey or later;
- Linux on AMD64 or ARM64; and
- Windows 10 or later on AMD64.

The launcher platform and the database deployment target are separate. A launcher running on any
supported platform can manage a cloud deployment.

## Local database

A local database requires:

- macOS 15 Sequoia or later;
- Apple Silicon; and
- at least 8 GB of host memory.

The database runs as a single node in a launcher-managed virtual machine. Local deployments are not
available on Linux or Windows in release 2.2.

## Cloud database

Cloud deployments run on AWS, Azure, Exoscale, or STACKIT infrastructure. The launcher provisions
Ubuntu 22.04 LTS on x86-64 compute instances.

The cloud account must have permission and quota for the selected compute, storage, and networking
resources. See the provider-specific account setup pages for details.
