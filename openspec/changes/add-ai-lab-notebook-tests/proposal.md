## Why

The `--with-ai-lab` capability installs the `exasol/ai-lab` container and pre-wires it to the database, but we have no automated way to confirm the shipped notebooks actually run against a real Exasol deployment. The only validation so far has been manual: deploy AWS, SSH into the host, run a notebook by hand, and read tracebacks off the screen. That loop is slow, expensive, human-in-the-loop, and easy to skip — and it is exactly how three Podman-vs-Docker integration bugs (missing `/var/run/docker.sock`, no unqualified-search registry, `DockerRegistryImageChecker` crashing on `"Already exists"`) and one upstream flavor-staleness issue (exasol/script-languages-release#1489) reached a live deployment before being noticed.

Every one of those failures is deterministic and would have been caught by executing the notebooks top-to-bottom in a clean environment. We want a headless "run-through" test harness that does this automatically — without an AWS deployment and without a human copy-pasting errors — so that future `exasol/ai-lab` and `notebook-connector` bumps are validated before they ship.

## What Changes

- Add a **headless notebook test harness** that starts the AI Lab container configured exactly as `installAiLab.sh` configures it (Podman socket mount, `registries.conf`, the `DockerRegistryImageChecker` patch, seeded SCS), brings up a self-contained database, executes the bundled notebooks with `papermill`, and reports a per-notebook result.
- Use the AI Lab's **integrated test database (ITDE / docker-db)** instead of a cloud deployment, so the harness runs locally or in CI with no AWS, no SSH, and no human in the loop.
- **Discover** notebooks at runtime (glob `*.ipynb` in the image) rather than hardcoding a list, so coverage tracks whatever the image ships and new notebooks are not silently missed.
- **Classify** each notebook result so failures are actionable, not just red/green: `PASS`, `FAIL (integration)` — our wiring is wrong, `FAIL (upstream)` — a defect in `ai-lab`/`notebook-connector`/a flavor, or `SKIP (needs <credential/resource>)` — requires an external secret or service we deliberately do not provide in the harness.
- Provide a **notebook coverage matrix** (in design.md) recording, for each notebook category, whether it is runnable unattended, and if not, what it needs.
- Optionally wire the harness into **CI** (GitHub Actions) as a regression gate that can be triggered on demand and on `ai-lab` / `notebook-connector` version bumps.

This change adds **test/verification tooling only**. It does not change the `--with-ai-lab` runtime behavior, the installed artifacts, or any user-facing surface.

## Capabilities

### New Capabilities
- `ai-lab-notebook-tests`: how the project headlessly executes the AI Lab's bundled notebooks against a self-contained database in a deployment-faithful container configuration, discovers notebooks automatically, and reports a classified per-notebook result suitable for use as a regression gate.

### Modified Capabilities
<!-- None. This is verification tooling for the existing ai-lab-access capability; it does not alter that capability's requirements. -->

## Impact

- **New test tooling** (location TBD in design, e.g. `tests/ai_lab/`): the papermill driver, the container/ITDE bring-up, the SCS seeding (reusing the same parameters as `installAiLab.sh`), the notebook-discovery + classification logic, and the report writer.
- **CI** (optional, `tasks/` + `.github/`): a workflow that runs the harness; not part of the default fast test suite because a full run is heavy (image pulls, ITDE startup, and — for `export_as_is` — SLC image builds).
- **No production code changes.** The harness consumes the same configuration that `installAiLab.sh` already applies; if anything, it documents that configuration as the single source of truth for "how a correct AI Lab host is set up."
- **External dependencies (test-time only):** a container runtime (Docker or Podman) and `papermill`. Credential-gated notebooks remain out of scope and are reported as `SKIP` rather than failures.
- **Known limits:** `export_as_is` currently fails on upstream flavor staleness (exasol/script-languages-release#1489) and will report `FAIL (upstream)` until that is fixed; heavy ML notebooks may exceed a small CI runner's resources and may be marked for a larger target.
