## 1. Secrets representation

- [x] 1.1 Extend shared secrets serialization with the optional service/bucket credential maps and explicit read/write identities; cover old/local documents and round trips with multiple services and buckets. Proposed commit: `feat: Represent BucketFS credentials by service and bucket`.

## 2. Cloud provisioning and installation

- [x] 2.1 Implement stable, distinct infrastructure-generated default-bucket passwords in all four cloud presets, owner-only secrets output, protected installation transport, and shared Ubuntu configuration before installation completes; include focused provider and installer tests for success, failure, retry, access-policy preservation, and confidential output. Proposed commit: `feat: Configure default cloud BucketFS credentials during installation`.

## 3. User workflow and behavioral evidence

- [x] 3.1 Document the nested credential fields, default cloud bucket and HTTPS upload workflow, local filesystem distinction, and new-deployment availability; update the net unreleased changelog and add appropriate launcher/live evidence for credential use and lifecycle stability. Proposed commit: `docs: Explain default cloud BucketFS access` (include directly related behavioral evidence in this implementation group).

## 4. Validation and review

- [x] 4.1 Run OpenSpec validation, relevant Go and installer/provider tests, required formatting and linting, and documentation validation; summarize evidence against each requirement. This task produces no commit.
- [ ] 4.2 Obtain live HTTPS credential and stop/start evidence in the user-authorized AWS CI run after the initial push. At archive time this evidence is pending; the user approved draft publication with that limitation. This task produces no commit.
- [x] 4.3 Complete an independent review using the repository rules, Jira intent, approved design, and validation evidence; resolve material findings within the approved scope. This task produces no commit.

## 5. OpenSpec completion

- [x] 5.1 Archive the change and sync its requirements after local validation and independent review, then prepare the user-authorized separate OpenSpec-only commit. Live CI verification remains pending as recorded in task 4.2.

The user authorized implementation, commits, push, draft PR publication after independent review, and an AWS deployment test in CI after push. Existing-deployment adoption, external rotation, and Jira writes need separate decisions. Live verification remains explicitly pending at archive time.

Local validation after rebasing onto current main passed: `task fmt`, `task docs-check`, `task all` including lint, build, Go race tests and 207 launcher tests (6 skipped), 14 focused provider/installer tests, and strict OpenSpec validation. Independent review of the initial and rebased snapshots found no confirmed defects. The new live test was collected but has not yet run against a real deployment.
