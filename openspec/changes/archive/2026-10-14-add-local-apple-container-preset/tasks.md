## 1. Preset and Runtime Selection

- [x] 1.1 Add `assets/infrastructure/local-apple-container/infrastructure.yaml` declaring `backend: local` and `compatibility.provides: [local-exasol]`
- [x] 1.2 Extend local runtime selection to construct the Apple Container runtime when `local-apple-container` is the selected preset, leaving platform-only selection for `local` unchanged
- [x] 1.3 Restrict the preset to macOS Apple Silicon running macOS 15 or later, and reject every other platform before starting a runtime, mirroring the existing unsupported-platform rejection

## 2. Environment Preparation

- [x] 2.1 Detect the `container` CLI and its system service; start the service without requesting approval when the CLI is already present
- [x] 2.2 Offer to install the `container` CLI through Homebrew when available, with explicit approval, reusing the existing host-change approval policy and `--auto-approve`
- [x] 2.3 When Homebrew is unavailable, offer to install the CLI from Apple's signed installer package instead, pinning its download URL and checksum in `assets/resources.yaml` the way other fetched tools are pinned
- [x] 2.4 Fail preparation with manual-install guidance when neither Homebrew nor the signed installer package is usable

## 3. Nano Lifecycle via `container`

- [x] 3.1 Implement image pull and `container run` argument translation for Nano, reusing the pinned image reference from `assets/resources/resources.yaml`; repack the pinned archive's directory entries to be traversable before loading it, since `container image load` (unlike Podman) rejects the 0600 directory mode the archive's docker:// source produces
- [x] 3.2 Reuse the shared generated-password bootstrap, host-visible `/exa` data layout, deterministic port allocation and bind-conflict handling, and bounded readiness wait
- [x] 3.3 Implement status and health reporting against the exact `container` container, and the stop/destroy lifecycle commands
- [x] 3.4 Reject script language container installation for this preset with an explicit, scoped error before attempting any mount
- [x] 3.5 Leave Nano without automatic restart on an unexpected stop; `exasol start` re-asserts desired state on the next invocation

## 4. Documentation and Verification

- [x] 4.1 Document `local-apple-container` as an explicit, opt-in alternative to `local`, including its macOS 15+ Apple Silicon requirement, both installation mechanisms, and its known limitations (no script language containers, no automatic restart)
- [x] 4.2 Cover environment preparation, approval flows, and preset platform gating with unit tests
- [x] 4.3 Extend the integration suite with a `local-apple-container` install-to-destroy lifecycle test, gated to macOS Apple Silicon runners (`tests/tests/live/test_local_runtime.py::test_install_local_apple_container_lifecycle`; no CI workflow changes needed, since `tests-deployment-local`'s existing self-hosted macOS row already runs the whole `tests/live` directory with `--infra=local --platform=macos-arm64`)
- [x] 4.4 Run repository unit tests, integration tests, lint, and build. `go build ./...`, `go vet ./...`, `gofmt`, the full `go test ./...` suite, `ruff`/`mypy` for the test and docs trees, and the strict `mkdocs` docs build all pass, with zero new failures compared to a baseline run on the same machine (pre-existing `/private/var` symlink and local-VM-runner-dependent failures are unrelated to this change). `golangci-lint` could not run in this environment: its bundled analysis tooling does not yet support this machine's Go 1.27.2 toolchain (a pre-existing, unrelated incompatibility, reproducible on an unmodified checkout) — re-run it in CI, which pins a compatible toolchain. The new integration test was verified by collection (`pytest --collect-only`) rather than by execution, since it needs a macOS Apple Silicon CI runner; the same install → connect → stop → start → destroy path it automates was exercised manually end-to-end through the built `exasol` binary against the real `container` CLI
