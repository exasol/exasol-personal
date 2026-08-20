## Context

The deployment-test workflow currently filters a declarative cloud plan but handles the local suite in one fixed macOS self-hosted job. The existing `os` input is documented as cloud-only, and the composite deployment-test action already supports the managed Python path used by hosted runners and the system-Python path required by the macOS runner.

## Goals / Non-Goals

**Goals:**

- Represent cloud and local jobs as independently filtered declarative plans.
- Reuse the current workflow inputs and local test task.
- Preserve the self-hosted macOS runner requirements and cloud authentication boundaries.
- Make unsupported exclusive selections fail before matrix expansion.
- Run portable local lifecycle coverage on both runtimes while reserving platform guards for macOS VM-specific behavior.

**Non-Goals:**

- Add Linux ARM64 local CI coverage.
- Add Windows local deployment support or an unsupported-platform-only lane.
- Run local deployment tests automatically on pull requests, pushes, or schedules.
- Duplicate CSV and additional connect coverage across local platforms.
- Require a custom-SLC source for every local workflow dispatch.

## Decisions

1. Use one planning job for both suites.

   The planner will filter separate cloud and local JSON plans and expose each matrix together with a `has_rows` output. This keeps suite/OS routing in one place and prevents an empty matrix from reaching job expansion. Separate planning jobs were considered but would duplicate selection and incompatibility handling.

2. Reuse the `os` workflow input.

   `ubuntu-latest` maps to the Linux AMD64 local row, `macos-latest` maps to the self-hosted macOS ARM64 row, and `windows-latest` has no local row. A new local-only input was rejected because it would complicate combined-suite dispatches and the existing trigger helper without adding needed expressiveness.

3. Store runner and setup metadata in each local plan row.

   The Linux row selects `ubuntu-latest` and managed Python setup. The macOS row carries the existing self-hosted label set and selects system Python. Both invoke the same local test task, so platform-specific workflow branching remains limited to matrix data.

4. Validate empty exclusive selections but permit empty halves of a combined selection.

   `suite=local` with a non-local OS fails in the planner. With `suite=all`, an OS may legitimately match only cloud or local rows, so the unmatched job is skipped through its `has_rows` output.

5. Preserve the current macOS commit-status context.

   The macOS row keeps `tests-deployment-local-self-hosted`; Linux receives a new platform-specific context. This avoids needless disruption to any repository settings that consume the existing status.

6. Select local tests by runtime capability.

   Portable lifecycle behavior runs on both Linux and macOS and asserts each runtime's published shell capability. Memory sizing, historical runner updates, and VM-daemon recovery remain guarded for the macOS VM runtime. COS-only diagnostics do not carry the local marker, and the obsolete unsupported-platform escape-hatch test is removed because Linux is supported and the launcher no longer implements that bypass. Custom-SLC cases remain selected but may skip when maintainers do not supply their manual test artifact.

## Risks / Trade-offs

- GitHub-hosted Ubuntu runner image changes can affect Podman behavior -> retain the repository's container-registry override and surface deployment diagnostics through the existing test action.
- A matrix value containing multiple self-hosted labels can be mishandled as a scalar -> represent every runner as an array and pass the matrix value directly to `runs-on`.
- Local tests are slow and resource-intensive -> retain manual dispatch and the protected deployment-test environment.
- Some local tests are macOS VM-specific -> retain their explicit skip decorators while keeping generic lifecycle coverage outside those guards.
- Optional custom-SLC coverage depends on a maintainer-supplied artifact -> retain its explicit skip when neither supported source variable is configured.

## Migration Plan

Update the workflow and documentation, validate selector combinations locally, then dispatch the Linux and macOS rows from a pushed branch. Rollback is a workflow-only revert to the single macOS job; launcher and deployment state are unaffected.
