## 1. Guidelines

- [x] 1.1 Rewrite the "Keep CLI output in the command layer" best-practice around the three output kinds and the CTA visibility rules.

## 2. Output mechanism

- [x] 2.1 Add a command-layer call-to-action helper that queues guidance to stderr and is suppressed only when `--json` is selected (not gated on an interactive terminal).
- [x] 2.2 Keep the stdout helper (primary output) and stderr-notice helper (operational notices) as-is.

## 3. Call-to-action migration

- [x] 3.1 Route the available-update hint (root pre-run) through the CTA helper.
- [x] 3.2 Move the `info` next-step guidance (documentation links and "Next steps") to the CTA helper; the whole block is decorative guidance.
- [x] 3.3 Move the `slc update` "run `exasol slc install`" hint to the CTA helper.
- [x] 3.4 Move the `init` "run `exasol deploy`" and "use `exasol config set`" hints to the CTA helper.
- [x] 3.5 Move the `config set` / `config reset` "run `exasol deploy`" apply guidance to the CTA helper.

## 4. Misrouting fixes

- [x] 4.1 Route `config set` / `config reset` effective-configuration output (and same-preset `init` patch) to stdout.
- [x] 4.2 Route the `cache unlock` confirmation to stderr as an operational notice.

## 5. Verification

- [x] 5.1 Update unit and integration tests to expect CTAs for text output (interactive or not) and never under `--json`.
- [x] 5.2 Add coverage for `config set` / `config reset` effective configuration on stdout.
- [x] 5.3 Add coverage for the `cache unlock` confirmation on stderr.
- [x] 5.4 Update `CHANGELOG.md` for the user-facing output changes.
- [x] 5.5 Run `task fmt`, `task lint`, and the full test suite.
