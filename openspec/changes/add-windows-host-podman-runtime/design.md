## Context

The Linux runtime already executes the complete local Podman installation policy directly on the host. Windows needs the same lifecycle, but host readiness additionally depends on Podman installation, Windows PATH propagation, and a running rootful Podman machine. The current runtime preparation call occurs inside deploy/start after workflow state has already changed, which is too late for interactive approval or safe cancellation.

## Goals / Non-Goals

**Goals:**

- Keep direct host container lifecycle behavior single-source across Linux and Windows.
- Isolate platform host preparation behind a narrow, idempotent boundary.
- Ask for host-changing approval in the command layer without corrupting JSON output or deployment workflow state.
- Preserve retryability after partial Windows prerequisite failures.

**Non-Goals:**

- Generalize the macOS VM runtime into the host runtime.
- Support Windows ARM64, host or container shells on Windows, or launcher-managed Windows VM sizing.
- Stop, resize, or delete the shared Podman machine during Exasol lifecycle operations.
- Replace the deployment-owned Nano data directory with a Podman named volume.

## Decisions

1. Generalize the Linux runtime and inject host-environment preparation.

   A single `HostRuntime` owns all common paths, Podman installation calls, endpoints, health, and shell capability behavior. It receives a narrow internal `hostRuntimeEnvironmentPreparer` with an idempotent `EnsureReady` operation. Linux and Windows constructors inject their respective preparers. This is preferred to OS branches inside lifecycle methods because tests can inject fakes and future direct-host platforms add preparation without multiplying platform conditions. A separate runtime remains appropriate when storage, command transport, or lifecycle semantics differ.

2. Prepare backends before recording operations in progress.

   The deployment backend gains an explicit preparation phase invoked under the deployment lock after workflow permission checks but before the in-progress state write. Deploy and start options carry local runtime preparation options. Tofu preparation is a no-op. This keeps declined prompts and prerequisite failures out of deployment-failed or interrupted states while leaving actual deploy/start failures unchanged.

3. Keep host-change presentation in the command layer.

   Runtime preparation emits typed host-change requests containing a change kind and exact commands. The command layer renders the explanation and prompt to stderr and supplies approval. `--auto-approve` returns approval without reading stdin. Non-interactive callers without that flag fail with actionable guidance. Raw winget and Podman-machine progress streams through a caller-provided stderr writer.

4. Reuse the previous Windows prerequisite policy with explicit command execution.

   Preparation first verifies `podman --version`, then refreshes PATH from machine and user registry values through PowerShell and retries. If still absent, approved setup invokes `winget install --exact --id RedHat.Podman --source winget --accept-source-agreements --accept-package-agreements`, refreshes PATH, and verifies Podman again. No new library dependency is needed.

5. Target the default Podman machine and require rootful operation.

   Preparation inspects `podman-machine-default`. A missing machine is initialized rootful with a fixed 40 GB disk and started; a stopped rootful machine is started. Converting an existing rootless machine requires a separate approval and uses explicit stop, set, and start commands. The provider remains Podman's default for compatibility with Podman versions that predate a provider flag.

6. Keep Windows data and runtime behavior aligned with Linux.

   Windows uses the deployment runtime directory as the `/exa` bind mount and executes `podman.exe` directly through the existing direct execution environment. The existing Windows AMD64 Nano artifact, SLC materialization, migration/recovery, configured database port, endpoint recovery, health probe, and unsupported shell errors are reused unchanged.

## Risks / Trade-offs

- Windows bind mounts can expose filesystem or path parsing differences -> cover exact generated arguments with unit tests and a live Windows lifecycle integration test.
- Windows Package Manager can fail transiently or require UAC -> preserve its output and causal error, leave workflow state unchanged, and make reruns safe.
- Converting the shared default machine can affect unrelated containers -> require explicit approval, show exact commands, and never stop or delete the machine during Exasol lifecycle operations.
- A generic host runtime could accumulate platform-specific behavior -> restrict the injected boundary to environment readiness and introduce a separate runtime if lifecycle or storage semantics diverge.

## Migration Plan

1. Introduce the generic host runtime and pre-workflow preparation framework without changing supported platforms.
2. Add and select Windows host preparation with focused tests.
3. Enable live Windows lifecycle coverage in the regular integration suite and update documentation.
4. Validate and archive the OpenSpec change after all commits pass lint and final verification.

Rollback reverts Windows selection and CLI approval plumbing while retaining the behavior-compatible generic Linux host runtime. Windows deployment data remains in the deployment directory and can be inspected or removed normally.
