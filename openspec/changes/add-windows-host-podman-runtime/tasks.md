## 1. Published Endpoint Reachability

- [x] 1.1 Publish the database port on the IPv4 loopback so WSL's localhost relay cannot expose an unreachable IPv6-only endpoint, and remove the container-level network workaround it replaces
- [x] 1.2 Remove the rootful Podman-machine conversion path, keep whatever privilege mode an existing machine has, and correct the Windows reachability guidance that advised converting

## 2. Shared Direct-Host Runtime

- [x] 2.1 Generalize the Linux host runtime into one direct-host runtime with an injected preparer contributing prerequisites, start-time re-checks, and the installation execution environment
- [x] 2.2 Expose the runtime's platform so diagnostics and platform-selection tests can distinguish direct-host platforms that share one runtime type
- [x] 2.3 Implement the Windows preparer: reuse an available Podman, refresh PATH from registered Windows values, install through Windows Package Manager with approval, and ensure the default machine runs
- [x] 2.4 Flush Nano's startup writes inside the Podman machine on Windows rather than on the host filesystem

## 3. Approval and Workflow State

- [x] 3.1 Replace runtime-owned prompting with declared host-change requests and a command-layer approval policy that denies when no approver is supplied
- [x] 3.2 Add `--auto-approve` to `deploy`, `install`, and `start`, and fail non-interactive runs that would otherwise proceed without approval
- [x] 3.3 Prepare the host before recording an operation in progress for every backend, with a no-op for cloud deployments
- [x] 3.4 Report preparation progress independently of the optional subprocess-output gate, without contaminating machine-readable lifecycle output

## 4. Platform Enablement

- [x] 4.1 Select the Windows host runtime for Windows AMD64 across all local workflows and list it as a supported local platform
- [x] 4.2 Treat direct-host platforms uniformly in local diagnostics, so Windows reports container status rather than VM status

## 5. Documentation and Verification

- [x] 5.1 Document Windows prerequisites, unattended approval, the shared Podman machine's lifecycle, and the user-visible approval and port-binding changes
- [x] 5.2 Cover the Windows preparer, the approval policy, and preparation-failure workflow state with unit tests
- [x] 5.3 Extend the integration suite with Windows platform gating, host configuration coverage, and a Windows install-to-destroy lifecycle test, and collect Windows diagnostics on CI failure
- [x] 5.4 Run repository unit tests, integration tests, lint, and build
- [ ] 5.5 Verify the Windows lifecycle on a Windows AMD64 runner, which the skip-gated integration test cannot exercise elsewhere
- [ ] 5.6 Confirm the database tolerates its data directory being reached across the Podman machine's filesystem passthrough, and decide whether the data must move inside the machine
- [x] 5.7 Add a Windows row to the local deployment test workflow
- [x] 5.8 Audit the per-test Windows skip markers in the local deployment suite, re-enable the ones that were only stale, and re-mark the pseudo-terminal cases with a reason describing what they actually require
