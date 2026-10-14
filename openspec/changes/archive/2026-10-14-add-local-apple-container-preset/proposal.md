## Why

macOS local deployments run Nano inside a launcher-owned helper VM (the downloaded `exasol-local-vm` runner) because Podman itself needs a Linux VM on macOS. Apple's `container` CLI already gives every container its own lightweight VM natively, through the Virtualization framework, which is the same isolation property that helper VM exists to provide. Manual testing confirmed `container` can pull and run the pinned Nano image, publish the database port, and connect with a generated `sys` password exactly like the existing local path.

Two gaps stand between that and parity with today's `local` preset: `container` has no `--mount type=image` equivalent, which is how Nano's script language containers (UDFs) are mounted today, and no `--restart` policy, which today's Podman install relies on. Both are real, scoped limitations rather than flag-translation details.

Rather than branch the existing `local` preset by platform capability, offer Apple's `container` as its own named, opt-in preset. That way a gap or defect in the new path cannot regress the supported `local` preset, and adoption is explicit.

## What Changes

- Add `local-apple-container` as a new, separately selectable local infrastructure preset (`exasol install local-apple-container`), independent of the existing `local` preset.
- Add a local runtime implementation that drives Nano through the `container` CLI directly on the host — no launcher-owned VM, managed share, or SSH-key import, unlike the macOS VM-backed runtime for `local`.
- Reuse the local deployment's existing generated-password, host-visible `/exa` data layout, deterministic port allocation, and bounded readiness-wait behavior unchanged, so the new preset feels identical to `local` wherever it legitimately can.
- Prepare the host by ensuring the `container` CLI and its system service are available, offering an install through Homebrew when present or Apple's signed installer package otherwise, with explicit approval either way, reusing the host-change approval policy (and `--auto-approve`) already generalized for Windows Podman installation.
- Explicitly reject script language container (UDF) installation on this preset with a scoped error, instead of attempting a lossy mount workaround.
- Document that Nano is not restarted automatically if it stops unexpectedly; recovery requires `exasol start`.
- Restrict the preset to macOS Apple Silicon running macOS 15 or later, rejecting other platforms before starting a runtime.

## Capabilities

### New Capabilities

- `local-apple-container-runtime`: Run Nano locally through Apple's `container` CLI as a separate, opt-in local preset, including environment preparation, standard local lifecycle behavior, and its scoped limitations (no script language containers, no automatic restart).

### Modified Capabilities

None. The existing `local` preset, its macOS VM-backed runtime, its Linux and Windows host runtimes, and their specs are unchanged.

## Impact

Adds a new infrastructure preset asset, a new local runtime implementation, and runtime-selection logic that chooses it only when `local-apple-container` is the selected preset. Adds environment preparation that installs the `container` CLI through Homebrew or Apple's signed installer package, reusing the existing host-change approval framework. Adds user documentation describing the preset as an explicit, opt-in alternative with disclosed limitations. Adds unit and integration test coverage gated to macOS Apple Silicon. Does not change the `local`, `aws`, `azure`, `exoscale`, or `stackit` presets, their backends, or their specs.
