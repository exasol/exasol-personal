# Design — Personal-owned local workloads

## Context

Personal currently delegates both virtualization and product workload behavior to an
embedded macOS runner. The refactor separates those responsibilities: local-vm v2 is a
replaceable macOS VM provider, while Personal owns Nano on every platform.

The existing deployment manifest and launcher state remain the only durable product
configuration. Workload specifications, adapter identities, Podman object IDs, and
generated runtime files are deliberately not serialized.

## Runtime model

Every local command reads the deployment manifest and launcher state and derives a fresh
`WorkloadSpec`. Read-only commands reconstruct without staging files or changing state;
mutating lifecycle commands regenerate disposable assets and reconcile live state.
Deterministic names and labels derive from the deployment UUID.

`MacVMAdapter` writes a versioned local-vm configuration, a workload helper and private
Kube manifest into a caller-owned control share. Its boot hook calls the helper's
`apply` mode. Later lifecycle and diagnostic operations call `apply`, `down`, `status`,
or `logs` over SSH. The adapter owns database readiness and uses local-vm state only as
a live VM observation.

`WindowsPodmanAdapter` uses the same manifest and helper semantics directly against the
current/default WSL2 Podman machine. It may offer user-scope Winget installation or
upgrade interactively, but never resizes, stops, removes, or changes the root mode of
an existing machine. `LinuxPodmanAdapter` reports that it is not implemented.

## Declarative workload

Personal applies one private Kubernetes Pod using `podman kube play --replace`. The
manifest pins the embedded architecture-specific image by digest, disables pulling,
mounts persistent `/exa`, supplies a 512 MiB memory-backed `/dev/shm`, requests
unlimited PIDs, uses an unmasked proc mount, always restarts, declares port 8563, and
passes the current initialization and version-check arguments. SLCs are read-only
image volumes on macOS arm64 and Windows amd64; each adapter ensures their images are
available before Kube Play.

SQL readiness remains the product contract. Kube probes are not a replacement.

## Data and storage

The data path is `<deployment>/local/data/exa`. VM and workload replacement never
remove it. On macOS, Personal shares `<deployment>/local/data` into the VM with
virtiofs and mounts `/mnt/data/exa` at `/exa`; on Windows it converts the same
canonical host path to its WSL-visible form. Only explicit deployment destruction may
remove Personal-owned data.

The provider's fixed sparse disk is internal scratch storage for the guest's writable
`/var`, including Podman state. It is not workload data or caller configuration.

## Legacy migration

Personal executes only the provider version pinned by the current launcher. A legacy
deployment is migration input, never a reason to invoke, preserve, or emulate a v1
runner. No adapter version, storage layout, port, or migration checkpoint is added to
launcher state.

When v2 initializes stopped pre-contract VM state, local-vm refreshes provider assets
without replacing the existing `/var` disk. Personal's root boot hook stops the legacy
Nano container, selects either `/var/lib/exa` or its overlay-backed `/exa`, copies it
to `<deployment>/local/data/exa.migrating`, verifies the copied tree, and atomically
renames it to `exa`. The target directory itself is the completion marker. Any failure
fails provider start and retains the source and staging data for diagnosis; a later
start can retry. Once `exa` exists, subsequent starts skip migration. Deployment
settings and the existing compatibility marker remain unchanged.

## local-vm v2 contract

The provider accepts `init/start --state-dir --config`, `stop/destroy --state-dir`,
`status/health-check --state-dir --json`, and `version --json`. Configuration, hook,
and state schemas are independently versioned. Caller paths are absolute, canonical,
and protected against traversal or symlink escape. Only configured TCP forwards exist;
an explicit unavailable port fails and zero selects a dynamic port.

The provider starts the root hook after Podman, shares, SSH, and forwarders are ready,
streams and records its output, and keeps the VM running after hook failure. `destroy`
removes only VM-owned state below `state-dir`; it never deletes caller shares.

## Release strategy

Personal development builds may embed a manually built v2 provider through
`RUNNER_PATH`. Release builds reject this override. The local-vm schemas and
packaging remain unpublished until macOS and Windows Personal validation is complete;
then local-vm v2.0.0 is published, Personal pins its URL and checksum, and release CI is
rerun against that artifact.

Hardware-validation builds require an explicit provider, launcher version, and
digest-pinned OCI image layout whose manifest, config, blobs, and target architecture
are verified before embedding. Trusted release CI instead exports the repository-pinned
Nano digests and embeds both supported archives automatically.
