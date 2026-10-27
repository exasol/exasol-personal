Each implementation task below maps to exactly one code-only commit that passes
the full check suite. Group 4 is validation and produces no commit.

## 1. Saved enabled state

- [x] 1.1 Add the optional `enabled` field as the first key of a saved
  definition, reject it in the catalog, save `enabled: true` on enable, save
  `enabled: false` on disable while keeping the entry, and report and reconcile
  only enabled entries in list, status, apply, and start. Report a repeated
  disable of a disabled entry as having nothing to remove. Verify `task tests-unit` covers the field round trip,
  the catalog rejection, enable on a new, disabled, and field-less entry,
  disable keeping the edited entry, repeated disable, and reconciliation
  removing a disabled sidecar's container. Verify `task tests-launcher` covers a
  field-less entry reporting as enabled and a disabled entry reporting as
  disabled in list and status. Add live tests for re-enabling an edited
  definition and for start removing a sidecar disabled in `sidecars.yaml`.

## 2. Removal with `--remove`

- [x] 2.1 Add `--remove` to `exasol sidecar disable`, backed by a deploy
  operation that deletes the saved entry and shares disable's cleanup, failure
  recording, and deferred behavior, and regenerate the CLI reference. Verify
  `task tests-unit` covers removal of an enabled and of a disabled entry and a
  repeated removal. Verify `task tests-launcher` covers removing an edited
  catalog sidecar and enabling it again from its template, and a repeated
  removal reporting nothing to remove.

## 3. Documentation

- [x] 3.1 Document the `enabled` field, disable keeping the saved definition,
  `--remove`, and the repeated-disable result in the user sidecar page, and
  update the existing unreleased sidecar changelog entry. Verify
  `task docs-check` passes.

## 4. Validation

- [x] 4.1 Run `task all` and confirm formatting, linting, unit tests and the
  launcher tests pass on the finished branch.
