## 1. Deployment Selection

- [x] 1.1 Resolve named and implicit default deployments through the shared
  new-then-legacy path lookup. Return path inspection errors and keep
  pre-registration aligned with command execution. Add Go tests for selected
  paths, precedence, and early lookup; verify with `task all`.
- [x] 1.2 Add launcher tests for a kept named deployment, a kept implicit
  default deployment, a collision, and an absent deployment selected for
  initialization. Check that status reports the selected path and legacy
  selection warns once on standard error.
  Audit every added or modified spec scenario for one clearly named test and
  verify with `task all`.

## 2. Deployment Listing

- [x] 2.1 Merge entries from both managed roots with new-location precedence
  and preserve status, sorting, and JSON output. Add Go and launcher tests for
  legacy-only entries and collisions; verify with `task all`.

## 3. User Guidance

- [x] 3.1 Update command help, deployment documentation, and the existing
  `Unreleased` changelog entry to explain the fallback and new-location
  preference in selection and listing. Verify the help output and
  documentation build with `task all`.
