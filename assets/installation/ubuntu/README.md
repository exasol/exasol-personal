# Exasol Ubuntu installation preset

This folder contains the **installation preset** for deploying Exasol on Ubuntu-based nodes.

An installation preset defines *how software gets installed and configured on already-provisioned nodes*. It is intentionally separate from the **infrastructure preset** (which defines *what nodes, disks, and network get created*).

## What this preset does (conceptually)

This preset implements a **recommended unattended installation style**:

- **Cloud-init** performs the one-time bootstrap (base packages, directory setup).
- **systemd** orchestrates the installation workflow as a sequence of ordered phases.
- The launcher primarily **monitors** progress; the installation continues even if the launcher disconnects.

The workflow is designed to work for single-node and multi-node deployments. Cluster-wide actions (installation coordination and post-install configuration) run only on the primary/access node, while preparatory steps run on all nodes.

## Workflow phases

At a high level, the node-local workflow is:

1. **Bootstrap** (cloud-init): prepare the OS and start the installation target.
2. **System preparation**: prepare the node so the installer can run reliably.
3. **User preparation**: establish node-to-node access required for cluster-wide operations.
4. **Node synchronization**: a barrier that ensures all nodes are ready before the cluster install begins.
5. **Installation**: run the cluster installation using the installer tooling.
6. **Readiness checks**: wait until the database and management services are operational.
7. **Post-install configuration** (optional per deployment): apply configuration that requires a running cluster (e.g., TLS and archive configuration).

The exact mechanics are intentionally owned by this preset; other presets may choose different phase boundaries or a different approach entirely.

## Inputs and interface contract

This preset expects machine-readable configuration to be present on each node:

- **Infrastructure description** (addresses, node count, credentials/material needed for installation)
- **Node identity** (e.g., which node is the access/primary node)

In practice these are written during bootstrap and consumed by the installation scripts.

This preset also assumes:

- Nodes can reach each other over the network during installation.
- Node-to-node SSH is possible (used for cluster-wide coordination).
- The base image provides cloud-init + systemd (so bootstrap and phase orchestration are available).
- A fixed node naming convention where the primary/access node is named **`n11`**.

## Observability and monitoring

The launcher surfaces progress by following a simple contract:

- The preset emits structured progress lines with stable prefixes:
  - `EXASOL-INSTALL-STEP:` for phase-level progress
  - `EXASOL-INSTALL-SUBSTEP:` for finer-grained updates
  - `EXASOL-INSTALL-ERROR:` for failures

This preset’s monitor streams cloud-init output and the systemd journal in a **resumable** way (cursor-based), so reconnecting does not require restarting the installation and does not lose important events.

## State and idempotency

To make the workflow restart-safe, phases are implemented as idempotent units:

- Each phase records completion via marker files in a dedicated state directory.
- Units are guarded so they do not re-run once a marker exists.

This keeps retries predictable (for example after a reboot) and enables the launcher to distinguish “still running” from “already completed”.

## Customization points

If you extend or fork this preset, typical customization points are:

- Phase ordering and responsibilities (systemd unit chain)
- The configuration schema written during bootstrap and consumed by scripts
- Post-install features (for example, which optional configuration is applied)
- Monitoring surface (which progress lines are emitted and how they are interpreted)

For the overall architecture and terminology, see `doc/architecture.md` and `doc/glossary.md`.

For the shared infrastructure↔installation contract (well-known paths, manifest schemas), see `doc/presets.md`.
