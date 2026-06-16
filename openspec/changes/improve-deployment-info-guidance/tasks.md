## 1. User-facing info behavior

- [x] 1.1 Make `exasol info` succeed for resolved deployment directories that are not initialized.
- [x] 1.2 Show the resolved deployment directory and `not_initialized` state before initialization.
- [x] 1.3 Add text next-step guidance for uninitialized, initialized, running, stopped, failed, interrupted, and in-progress states.
- [x] 1.4 Preserve lifecycle semantics: `info` reports state and guidance without mutating deployment state.

## 2. JSON deployment overview

- [x] 2.1 Make `exasol info --json` return valid JSON for uninitialized deployment directories.
- [x] 2.2 Include deployment state and resolved deployment directory in JSON output.
- [x] 2.3 Include initialized deployment metadata and connection-relevant fields when available.
- [x] 2.4 Keep terminal-only guidance prose out of JSON output.

## 3. Verification

- [x] 3.1 Add coverage for not-initialized text output.
- [x] 3.2 Add coverage for not-initialized JSON output.
- [x] 3.3 Add coverage for initialized/running JSON output with connection details.
- [x] 3.4 Add coverage for deployment states where connection details are absent or not stable.
- [x] 3.5 Run the repository validation used for the pull request.
