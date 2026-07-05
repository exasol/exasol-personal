## 1. Lifecycle JSON Contract

- [x] 1.1 Add a lifecycle completion output type and JSON renderer for `running`/ready and `stopped`/not-ready outcomes.
- [x] 1.2 Register the existing `--json` output flag on `start` and `stop`.
- [x] 1.3 Route successful `start --json` and `stop --json` to emit exactly one JSON document on stdout.

## 2. Output Compatibility

- [x] 2.1 Keep non-JSON `start` behavior, including final connection instructions, unchanged.
- [x] 2.2 Suppress start connection instructions from stdout when JSON mode is selected.
- [x] 2.3 Ensure existing logging continues to use stderr or deployment log routing, not lifecycle JSON stdout.

## 3. Tests and Validation

- [x] 3.1 Add focused Go tests for lifecycle JSON rendering and flag registration.
- [x] 3.2 Add or update integration coverage for `start --json` and `stop --json` using the local fake runtime path.
- [x] 3.3 Run formatting, focused tests, and repository validation appropriate for this change.
