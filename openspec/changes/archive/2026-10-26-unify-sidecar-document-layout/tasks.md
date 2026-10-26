Each implementation task below maps to exactly one code-only commit that passes
the full relevant check suite. Group 3 is validation and produces no commit.

## 1. Shared definition shape

- [x] 1.1 Add the `Definition` type holding the container a sidecar runs and
  embed it inline in the catalog `Entry` beside the description and
  architectures. Verify `task tests-unit` passes with the catalog resolving a
  container through the embedded definition.

- [x] 1.2 Replace the saved document's `containers` list with a `sidecars` map
  of definitions keyed by sidecar name, derive an absent container name from
  its key, reject a `container.name` that contradicts its key, report
  validation locations as `sidecars.<name>.container...`, expose the document's
  names in sorted order, and update saved document reading and writing, the
  sidecar operations, the Python test framework, and the launcher tests that
  write `sidecars.yaml`. Verify `task tests-unit` and `task tests-launcher`
  pass, with one new test for an entry whose container omits `name` and one for
  an entry whose `container.name` contradicts its key, alongside the existing
  coverage of enable, repeated enable, catalog changes, status in name order,
  and disable of an unsaved name.

## 2. Documentation

- [x] 2.1 Update the saved document example and surrounding text on the user
  sidecar page, update the catalog and saved document description on the
  developer sidecar page, and update the existing unreleased changelog entry
  for sidecars. Verify `task docs-check` passes.

## 3. Validation

- [x] 3.1 Run `task all` and confirm formatting, linting, unit tests and the
  launcher tests pass on the finished branch.
