## Context

The repository currently separates Python black-box tests into `integration`, `deployment`, `e2e`, and `chaos` directories, then overlays kind and provider markers. A session-scoped `reusable_deployment` is shared by read-only, lifecycle-mutating, and recovery tests. Task commands and CI encode a partly declarative cloud matrix for AWS, Azure, and Exoscale plus local Linux AMD64 and macOS ARM64 rows, but successful runs do not consistently explain which evidence was omitted. STACKIT remains a supported preset but is excluded from live coverage until its deployment path works correctly.

The merged testing strategy is authoritative for why and what the repository tests. This design turns that strategy and the attached architecture proposal into an implementation plan. Product-facing OpenSpec scenarios require end-to-end evidence as a review principle; mandatory one-to-one identifiers and automated enforcement are deferred.

## Goals / Non-Goals

**Goals:**

- Make behavior ownership, execution boundary, state ownership, provider applicability, and cost independently understandable.
- Keep the default pull-request suite deterministic, unprivileged, cloud-free, and within the roughly 15-minute feedback target.
- Preserve real local and cloud evidence for claims that controlled tests cannot prove.
- Make shared, reusable, and isolated live deployments explicit and prevent accidental order dependence.
- Make selected and omitted live evidence visible in local output and CI summaries.
- Deliver one complete cutover to the new framework without retaining a compatibility taxonomy or transition contract.

**Non-Goals:**

- Changing pytest or introducing another Python test framework.
- Changing launcher behavior or Go test scope.
- Reimplementing provider or installation behavior in a fake Python backend.
- Comprehensive testing of Exasol Database, AdminUI, or other deployed products.
- Requiring every live scenario to run against every provider or host platform.
- Enforcing automated one-to-one OpenSpec scenario identifiers in this change.

## Decisions

### Separate behavior ownership from execution boundary

Fast tests will be grouped by launcher capability or user journey, while live tests will be grouped by lifecycle, introspection, interaction, and narrow cloud/local smoke responsibilities. The directory answers what owns the behavior; fixtures answer what environment it requires. This replaces overlapping directory and kind-marker taxonomies without treating each launcher command as an isolated test unit.

Alternative considered: retain the existing directories and improve marker documentation. This leaves two competing classification systems and keeps deployment cost invisible at the test call site.

### Use a small fixture vocabulary

The test-facing fixtures will be:

- `deployment`: compiled launcher with controlled state and no-resource presets;
- `local_deployment`: a real supported local runtime;
- `shared_live_deployment`: one ready deployment for lifecycle-read-only tests;
- `reusable_live_deployment`: one exclusive deployment for vetted mutations that can be restored to ready state;
- `isolated_live_deployment`: a live deployment owned by a mutating or destructive scenario.

Lifecycle state preparation, readiness polling, cleanup, diagnostics, command logging, and secret redaction belong to the framework. The finished repository exposes only this fixture vocabulary and the replacement task and selection contract; legacy task names, selectors, and marker taxonomy are removed as part of the cutover.

Scenarios that require exceptional setup, including interrupted provisioning, custom configuration, historical-launcher handoff, or a module-scoped local journey, may use a narrowly named test-local fixture backed by the deployment manager. The manager remains responsible for resource ownership and cleanup, so these exceptions do not add another general-purpose execution boundary.

Alternative considered: encode boundary and provider entirely through pytest markers. Markers are easy to combine inconsistently and do not make state ownership visible in a test signature.

### Keep shared and reusable live deployment state stable

Tests that inspect status or connection metadata and repeatable interaction tests may use the shared fixture. Vetted tests that stop, start, redeploy, or interrupt completed operations may use the reusable fixture when their postcondition is a ready, connectable database. The reusable fixture runs exclusively on one xdist worker, restores that postcondition after every consumer, and destroys the deployment when restoration fails so later tests cannot inherit contaminated state. Tests that destroy, reconfigure, install software, or deliberately corrupt state use an isolated fixture.

Alternative considered: restore one shared deployment after every mutation. Rejected because unrestricted restoration can hide state leakage and make later failures depend on cleanup quality and execution order; reuse is limited to explicitly vetted scenarios behind a separate fixture.

### Keep provider policy in Task and CI configuration

Tests describe exceptional provider and platform restrictions through local pytest annotations. Task and CI configuration select suites, providers, platforms, and jobs without a separate inventory. AWS remains the broad live-behavior provider initially because the current workflow already runs the full deployment suite there; AWS, Azure, and Exoscale receive focused provider smoke coverage. STACKIT is reported as omitted, with the reason that its deployment path does not currently work correctly, until it can join the smoke coverage. Local release validation retains Linux AMD64 and macOS ARM64 rows defined by the existing local-deployment workflow specification.

Alternative considered: mark every test with its CI providers. Provider annotations remain exceptions because repeating CI policy on every test creates another inventory.

### Report evidence boundaries explicitly

Pytest and CI summaries will state whether fast, local, live, provider-specific, stress, and chaos evidence ran, together with selected and omitted providers or platforms and the number of deployments created. A successful partial run remains successful but cannot present itself as complete live validation.

Alternative considered: rely on job names and command lines. Those are difficult to interpret after matrix filtering and do not expose omitted evidence.

### Treat OpenSpec traceability as a review contract

Capability inventories and change reviews will identify the relevant product specification and end-to-end evidence where applicable. New or changed product-facing scenarios normally add or update evidence. This change does not introduce scenario IDs, coverage percentages, or an enforcement tool; those can be proposed later if manual traceability proves insufficient.

Alternative considered: require a machine-enforced one-to-one map immediately. Existing specifications and tests were not authored with stable shared identifiers, so mandatory enforcement would expand the migration beyond the Epic's appetite.

## Risks / Trade-offs

- [Controlled fixtures diverge from live behavior] → Keep controlled boundaries narrow and update their contract tests when a live defect exposes an unrealistic assumption.
- [The full cutover drops behavior] → Reconcile every existing scenario with an end-state suite before deleting the legacy structure, then validate the finished selectors as a complete contract.
- [A canonical provider hides portability defects] → Retain critical smoke paths for AWS, Azure, and Exoscale, keep provider-specific applicability explicit, and report STACKIT as omitted until its deployment path works correctly.
- [Shared or reusable state remains flaky] → Restrict sharing to read-only behavior, restrict reuse to vetted restorable mutations, and destroy reusable deployments on failed restoration.
- [Reporting adds CI complexity] → Generate summaries from collected test annotations and the selected CI inputs.
- [The 15-minute target encourages skipped evidence] → Separate pull-request and release evidence explicitly instead of treating the fast suite as complete validation.

## Delivery

The change is delivered as one complete framework replacement. The finished repository uses the new fixture vocabulary, capability-oriented directories, test-local annotations, task commands, CI selection, and evidence summaries exclusively. Legacy directories, kind markers, selectors, and aliases are removed before the refactor is considered complete. Intermediate working-branch states are neither supported nor documented as a migration contract.

## Open Questions

None. Provider breadth can be adjusted through Task and CI configuration without changing this architecture.
