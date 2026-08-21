## Why

The manual deployment-test workflow runs real local deployment tests only on the macOS ARM64 self-hosted runner, so the supported Linux host runtime is not exercised end to end in GitHub Actions. The workflow needs selectable Linux AMD64 and macOS ARM64 local lanes without enabling unsupported Windows or deferred Linux ARM64 coverage.

## What Changes

- Run the local deployment test suite on GitHub-hosted Linux AMD64 and the existing self-hosted macOS ARM64 virtualization runner.
- Apply the existing OS workflow selector to local as well as cloud test rows while preserving the current cloud test plan.
- Reject an exclusively requested local platform when no supported local row matches, while allowing the combined suite to run whichever rows match.
- Keep the workflow manually dispatched, run portable lifecycle coverage on both local runtimes, and preserve skips only for macOS VM-specific behavior.
- Exclude tests that are incompatible with local deployments from the local suite while retaining manually supplied custom-SLC coverage.
- Treat transient connection resets from non-VM local runtimes during database startup as refused connections so readiness waiting can continue.
- Document the expanded local deployment test coverage and runner behavior.

## Capabilities

### New Capabilities

- `local-deployment-test-workflow`: Manual selection and execution of real local deployment tests on the enabled Linux AMD64 and macOS ARM64 runners.

### Modified Capabilities

None.

## Impact

The deployment-test GitHub Actions workflow, local test selection and platform guards, its shared composite action inputs, non-VM local-runtime reachability classification, Task descriptions, and CI/testing documentation are affected. Public CLI behavior, cloud credentials, and automatic pull-request CI are unchanged.
