## Context

Today's local deployment selects one of three runtimes purely by host platform: a macOS VM-backed Podman runtime, a Linux host Podman runtime, or a Windows host Podman runtime, all behind the single `local` infrastructure preset. The macOS VM exists specifically because Podman needs a Linux VM on macOS; the launcher downloads and manages its own VM runner (`exasol-local-vm`) to provide one.

Apple's `container` CLI (apple/container on GitHub) gives every container its own lightweight VM natively via the Virtualization framework — the same isolation property the launcher's helper VM exists to simulate. Manual testing against the pinned Nano image (`docker.io/exasol/nano`, digest from `assets/resources/resources.yaml`) confirmed `container image pull` and `container run` work, the database port publishes correctly, host-visible `/exa` data matches what Podman produces, and a generated `sys` password connects successfully.

That testing also surfaced two gaps against `container run --help` and the project's own docs: no `--mount type=image` (or equivalent) for mounting an OCI image directly into a running container, which is how Nano's script language containers are mounted today (`--mount type=image,source=<slc-image>,rw=true`), and no `--restart` policy of any kind.

## Goals / Non-Goals

**Goals:**

- Offer Apple's `container` as a separately named, explicitly opt-in local preset.
- Reuse the existing local deployment's generated-password, host-visible data layout, deterministic port allocation, and bounded readiness-wait contracts unchanged, so the experience matches `local` wherever the underlying capability allows it.
- Leave the `local` preset's code paths, specs, and tests completely unmodified.
- Make the two known capability gaps explicit, scoped failures rather than silent or partial behavior.

**Non-Goals:**

- Script language container (UDF) support for this preset in this iteration.
- Automatic restart of a crashed Nano container.
- Intel Macs, Linux, or Windows support for this preset.
- Replacing, deprecating, or becoming the default for the existing `local` preset.

## Decisions

1. **A new preset name, not a new branch of `local`.**

   `local` already dispatches to one of three runtimes by platform. Adding `container` as a fourth branch there would mean a defect or capability gap in the new path risks the default, supported local experience. A separate preset name (`local-apple-container`) makes adoption explicit and keeps the blast radius of this change contained to users who choose it.

2. **A direct-host runtime shape, not a VM-backed one.**

   `container` manages its own per-container VM, so the new runtime needs no launcher-owned VM, managed share, or SSH-key import — it looks like the Linux/Windows direct-host runtime (shell out to a CLI on the host), not the macOS VM-backed one. This is less code than the path it parallels on the same OS.

3. **Reuse the existing host-change approval framework.**

   The Windows Podman work generalized host-change approval into a command-layer decision (`--auto-approve`, declared commands, deny-when-no-approver) that is not Windows-specific. Installing the `container` CLI reuses that framework rather than inventing new prompting; starting its already-installed system service is an approval-free re-check, matching how an existing Podman machine is restarted without approval.

4. **Support installation without assuming Homebrew.**

   Not every macOS user has Homebrew, so it cannot be the only installation path. Apple also publishes a signed installer package (`.pkg`) as a release asset on `apple/container`'s GitHub releases. The launcher prefers Homebrew when it is present, since that mirrors how Podman itself is typically obtained, and falls back to downloading the pinned installer package otherwise, verified against a recorded checksum the same way other resources in `assets/resources.yaml` are pinned — never an unauthenticated "fetch latest release" call.

5. **Reject script language containers explicitly, rather than working around the mount gap.**

   `container`'s `--mount` only supports `bind`, `volume`, and `tmpfs`. A copy-based workaround (extract the SLC image into a bind-mounted directory) would change the correctness guarantee the current Podman mount deliberately provides — never modifying the image, keeping writes in an ephemeral overlay. Rather than ship a lossier mechanism, SLC installation fails with a scoped, explicit error until an upstream equivalent exists.

6. **No restart-policy equivalent.**

   `container run`/`create` has no restart flag at all. Building launcher-side supervision (a background watcher that restarts a stopped container) is a meaningfully different commitment than delegating to the container runtime, and is out of scope here. `exasol start` still re-asserts the desired state; an unexpected stop between invocations is not self-healing, and this is disclosed rather than hidden.

## Risks / Trade-offs

- `apple/container` is young (first release ~May 2025) and macOS-only, with its own roadmap; this preset's implementation may need rework as the tool matures.
- macOS 15 has documented `vmnet` limitations (no container-to-container isolation nuance, single network, a subnet-assignment race on the first container start); macOS 26 is recommended, and this preset's own readiness/reachability diagnostics need to account for that rather than reusing `local`'s macOS VM guidance verbatim.
- Two capabilities are deliberately missing versus `local` (script language containers, automatic restart). This is a real UX gap that must be visible in the preset's description and documentation, not just this spec, so a user does not discover it mid-task.
- Running a second local container-runtime integration long-term increases maintenance surface. Acceptable while the preset is opt-in and clearly scoped; worth revisiting only if adoption and `container`'s maturity justify promoting it further.
- The pinned Nano image archive, produced by resolving its `docker://` source, carries directory entries (`blobs/`, `blobs/sha256/`) with mode `0600` — no execute bit, so the directory cannot be traversed. Podman's own loader tolerates this; `container image load` and the standard `tar` command do not, and fail with a permission error partway through the first blob. Found during live end-to-end verification, not anticipated at design time. `AppleContainerInstall` now repacks the archive with the owner execute bit added to directory entries before handing it to `container image load` (`repackWithTraversableDirectories`); the pinned archive itself and the Podman path are unchanged.

## Migration Plan

None required. This is a new, separately named preset; it has no relationship to existing `local` deployments, their directory layout, or their data. Nothing about an existing deployment changes.

## Open Questions

- Should `--mount type=image` (or an upstream equivalent) land in `apple/container` before script language container support is attempted, or should the launcher build a copy-based mount workaround sooner despite the weaker guarantee?
- At what point, if any, would promoting `local-apple-container` to be the default `local` behavior on macOS be reconsidered?
