## 1. Configuration

- [x] 1.1 Add the embedded catalog, typed Container subset, strict validation,
  and architecture resolution. Resolve selections through an ordered resolver
  so entries assembled at runtime share the catalog's validation and defaults
  and take precedence over embedded templates. Verify catalog lookup,
  defaults, reserved names, runtime-entry precedence and validation, and
  invalid-field tests with `task tests-unit`.
- [x] 1.2 Add atomic deployment definition storage and catalog-to-deployment
  materialization under the deployment lock, with access restricted to the
  deployment owner. Verify first enable, preserved edits, file permissions,
  and empty-file behavior with `task tests-unit`.
- [x] 1.3 Add connection-reference resolution and Kubernetes expansion.
  Verify required and optional references, expansion ordering and escaping,
  and saved references with `task tests-unit`.

## 2. CLI

- [x] 2.1 Wire `sidecar list`, `enable`, `status`, and `disable`, including
  deployment selection and `--json`. Report deployment enablement with an
  empty host array, route recovery guidance through the CTA
  helper, and register deployment-file logging for actions. Add command tests
  and the unreleased changelog entry. Regenerate the CLI reference with
  `task docs-cli-reference`. Verify `task tests-launcher`, `task docs-check`,
  and `task docs-build`.

## 3. Documentation

- [x] 3.1 Document the commands, saved configuration, and connection
  references for users, and catalog entries and command-supplied templates
  for contributors. Verify `task docs-check` and `task docs-build`.

## Commit and completion boundaries

Each numbered task is one independently verifiable commit. Keep updates to
existing behavior documentation with their implementation. Run the full
relevant checks for each commit. Keep all files in this active change
uncommitted and update checkboxes locally as work completes.

After all implementation tasks pass, sync the delta spec, archive this change,
and make the pure OpenSpec result the final branch commit. Preserve the user's
one-line Conventional Commit format.

## Scenario-to-test ownership

Sidecar configuration scenarios are ADDED. Each has exactly one owning test,
identified for review with the `openspec` marker on launcher tests.
Parameterization remains an instance of that test.

### sidecar-configuration

- Catalog listing describes deployment availability:
  `TestSidecarListReportsAvailability` (command unit).
- Unsupported selection explains the available choices:
  `TestSidecarEnableReportsSupportedChoices` (command unit, parameterized).
- First enable materializes the template:
  `TestSidecarEnableMaterializesTemplate` (deployment unit).
- Repeated enable preserves deployment edits:
  `TestSidecarEnablePreservesEdits` (deployment unit).
- Catalog changes preserve enabled definitions:
  `TestSidecarSavedOperationsIgnoreCatalog` (command unit).
- Repeated disable reports the sidecar disabled:
  `test_sidecar_saved_definition_commands` (launcher).
- Invalid configuration identifies the offending location:
  `TestSidecarInvalidConfigurationLocation` (configuration unit, parameterized).
- Broad publication request receives a validation error:
  `TestSidecarLoopbackPublicationValidation` (configuration unit).
- Saved connection entries remain references:
  `TestSidecarEnableStoresConnectionReferences` (deployment unit).
- Expansion preserves Kubernetes escaping and ordering:
  `TestSidecarKubernetesExpansion` (resolver unit, parameterized).
- Required connection reference is unavailable:
  `TestSidecarRequiredReferenceError` (resolver unit).
- Saved definitions are owner-readable only:
  `TestSidecarEnableMaterializesTemplate` (deployment unit).

### Fixture and execution details

Launcher tests run without provisioned infrastructure. Catalog discovery is
covered through the command surface with an injected catalog, so one binary
serves every test. Runtime delivery of resolved values is specified by the
lifecycle capability that a later change introduces.
