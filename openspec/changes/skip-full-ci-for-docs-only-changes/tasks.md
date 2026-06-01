## 1. Workflow Classification

- [x] 1.1 Add a CI classifier job that compares changed files for pull requests and requires full CI for main-branch push events.
- [x] 1.2 Define the documentation-only path allow-list.
- [x] 1.3 Expose classifier outputs for documentation quality and full CI requirements.
- [x] 1.4 Move the full CI exemption list into a root `.ciignore` file.
- [x] 1.5 Protect `.ciignore` through CODEOWNERS.

## 2. Conditional Validation

- [x] 2.1 Add a documentation quality job that runs when documentation files changed.
- [x] 2.2 Gate Go linting, Python linting, build, unit test, and integration test jobs on the full CI classifier output.
- [x] 2.3 Add a final aggregate CI job that validates all classifier-required jobs succeeded.

## 3. Documentation and Verification

- [x] 3.1 Document that branch protection should require the final aggregate CI job.
- [x] 3.2 Validate the workflow syntax.
- [ ] 3.3 Update repository branch protection or rulesets to require the final aggregate CI job.
