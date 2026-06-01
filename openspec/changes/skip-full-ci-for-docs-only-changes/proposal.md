## Why

Documentation-only pull requests currently run the full CI pipeline, which spends build and test resources on changes that cannot affect runtime behavior. The repository also needs a path to add Markdown and spelling checks without forcing documentation contributors through the full code validation pipeline.

## What Changes

- Add CI change classification based on changed file paths.
- Run documentation quality checks for documentation-only changes.
- Skip Go, Python, build, unit test, and integration test jobs when all changed files are documentation-only.
- Keep full CI mandatory whenever at least one changed file is outside the documentation-only path rules.
- Add one final required CI gate that represents the effective result for branch protection.

## Capabilities

### New Capabilities

- `ci-change-classification`: CI behavior for classifying pull request changes and selecting the required validation stages.

### Modified Capabilities

None.

## Impact

- Affects the pull request CI workflow in GitHub Actions.
- Requires repository branch protection or rulesets to require the final CI gate instead of individual jobs that may be intentionally skipped.
- Does not change application code, runtime behavior, release automation, or deployment tests.
