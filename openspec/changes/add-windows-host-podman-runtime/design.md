## Context

The Linux host runtime already drives host Podman directly and shares one installation policy with the macOS VM runtime. Windows can reuse that policy unchanged, but differs from Linux in two ways that a single runtime type has to absorb: its prerequisites are not usually satisfied and installing them mutates shared host state, and its containers run inside a WSL2 Podman machine whose kernel is not the host's.

The approval question is what forced the wider refactor. A runtime that prompts for itself has to know whether it is attached to a terminal, and a runtime that prepares after the workflow has recorded an operation in progress cannot fail cleanly.

## Goals / Non-Goals

**Goals:**

- Keep one direct-host runtime, with platform differences confined to an injected seam.
- Make host-change approval an explicit command-layer decision that fails safe.
- Keep declined or failed prerequisites retryable.
- Leave shared host state — an existing Podman machine's configuration, its running state after destroy — as the launcher found it.

**Non-Goals:**

- Support Windows ARM64.
- Manage more than Podman's default machine, or select its provider.
- Provide host or container shells on Windows.
- Install Podman on Linux, where it remains a documented prerequisite.

## Decisions

1. Inject platform differences through a preparer rather than branching a shared runtime.

   One `HostRuntime` owns the container lifecycle. A per-platform preparer contributes the platform's prerequisites, the approval-free re-checks that apply at start, and the execution environment installation commands run through. Two things differ between Linux and Windows, not one, so a seam covering only prerequisites would have forced the Windows-only durability behavior back into the shared type.

2. Report the platform rather than distinguishing runtimes by type.

   Collapsing two runtime types into one removes the type assertion callers used to select platform-specific guidance. The runtime exposes its platform instead, so diagnostics keep separate Linux and Windows advice and platform-selection tests keep discriminating between them.

3. Separate approval-gated preparation from approval-free re-checks.

   Installing Podman changes state shared beyond the deployment and can raise an elevation prompt, so it requires approval. Creating or starting Podman's default machine is how Podman is normally used, and starting a machine that stopped between prepare and start needs no second decision. Only the install is gated; the machine check runs again at start without one.

4. Let runtimes declare host changes as data and let the command layer decide.

   A runtime returns the kind of change and the exact commands it intends to run. The command layer renders them, applies `--auto-approve`, and detects whether it can ask at all. This keeps terminal detection and prompt wording out of the runtime, and guarantees the user is shown the same command that runs.

5. Deny host changes when no approver is supplied.

   A missing approver is a wiring mistake, not permission. Treating it as denial means a caller that forgets to supply one fails loudly instead of mutating the host, and non-interactive runs refuse rather than assume consent. `--auto-approve` is the only way to approve without being asked.

6. Prepare the host before recording an operation in progress.

   Preparation moves ahead of the workflow-state write for every backend, with cloud deployments contributing a no-op. A declined install or a missing prerequisite then leaves the deployment in whatever state it already had, so the user can approve and retry rather than first recovering an interrupted operation.

7. Give preparation progress its own output channel.

   Preparation reports multi-minute steps the user needs to see, while `--verbose` governs optional subprocess output. Preparation writes to a progress writer supplied alongside the approver instead of to the `--verbose`-gated streams, which keeps prompts and download progress visible without widening what `--verbose` means.

8. Publish the database port on the IPv4 loopback.

   Podman's rootless networking creates a dual-stack published listener. WSL mirrors that to the host as an IPv6-only endpoint, and cannot forward it to a container listening on IPv4, so clients that prefer IPv6 connect and are then reset. Binding the loopback address explicitly keeps the relay on IPv4. The binding is narrower on every platform, which matches what a local deployment already documents.

9. Flush startup writes inside the Podman machine.

   The deployment's data directory is a host path mounted into the container, so on Windows the bytes land on the Windows filesystem rather than inside the machine. The writes are still buffered by the machine's kernel on the way out, and Windows offers no host-side `sync` the launcher could invoke, so the durability workaround runs inside the machine. It removes the machine's buffering from the path; it does not prove the Windows filesystem has committed.

## Risks / Trade-offs

- Non-interactive host preparation now fails where it previously proceeded. This is the intended correction, but it is a behavior change for any existing scripted invocation that relied on the old default; `--auto-approve` is the documented replacement.
- Windows behavior is verified by unit tests and a skip-gated integration test, so it depends on a Windows CI run rather than on local verification.
- The launcher shares Podman's default machine with everything else on the host. It therefore never reconfigures the machine and leaves it running after destroy, at the cost of not reclaiming those resources itself.
- The database's data directory is inherited unchanged from the Linux host runtime, which means it is a host path on Windows too. The container therefore reaches it across the Podman machine's filesystem passthrough rather than through the machine's own disk, which is the arrangement macOS deliberately avoids by keeping the data inside its VM.

  That passthrough is the least-verified part of this change. A database is a demanding filesystem client, and a passthrough mount offers weaker throughput for small operations and weaker `fsync`, locking, and sparse-file guarantees than a native filesystem. Consequences range from acceptable-but-slow to the database failing to initialize. The SELinux relabel suffix carried on the mount is also meaningless there, and may be rejected rather than ignored.

  Keeping the data on the host is what makes the deployment directory self-contained, portable, and removable without entering the machine, so it is not obviously the wrong choice. But if the passthrough proves unsuitable, the fix is to move the data inside the machine as macOS does, which changes what `destroy` has to reach into and how existing deployments are adopted. This is deliberately left for the first real Windows run to answer rather than pre-emptively redesigned.

## Migration Plan

No state or data conversion is required. Existing deployments keep their directory layout and their `exa` data, and are adopted in place on the next start.

Two behaviors change for existing deployments the first time they are started with the new launcher:

1. The database container is recreated with the narrower published port, so a deployment previously reachable from another host becomes loopback-only. Users who relied on that reach need a cloud deployment.
2. Host preparation now requires approval. On Linux this is unreachable in practice, because Podman remains a documented prerequisite rather than something the launcher installs; on Windows a scripted run needs `--auto-approve`.

Rollback is downgrading the launcher: the previous version republishes on the wildcard address and restores the prior approval behavior, with no data-format reversal. A Windows deployment created by the new launcher cannot be operated by an older one, which has no Windows local support.

## Open Questions

- Is the Podman machine's filesystem passthrough an acceptable home for the database's data directory, or must the data move inside the machine as it does on macOS? This is the change's main unresolved risk and is tracked as a verification task rather than decided here.
- Do the newly re-enabled local tests actually pass on Windows? Of the 27 tests the local suite selects, 22 are now expected to run there, up from 5. The stale skips came from a time when Windows had no local deployment, so removing them is correct in principle, but none of those tests has ever executed on Windows and the first run should be read as discovery rather than regression.

