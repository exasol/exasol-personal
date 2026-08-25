## 1. Windows Host Preparation

- [x] 1.1 Simplify default-machine preparation, initialize without a mode flag, start stopped machines, and remove privileged-runtime conversion and prompt code; verify affected Go packages compile without mode-specific symbols
- [x] 1.2 Update focused Windows preparer tests for mode-independent no-op, initialization, and start behavior while removing conversion-only cases; verify the focused cases with `go test ./internal/localruntime -run '^(TestWindowsHost|TestMergeWindowsPath)'`
- [x] 1.3 Remove the Windows lifecycle test's rootful-mode assertion while retaining machine-readiness coverage; verify the edited Python test module passes static syntax checking
- [x] 1.4 Use `podman machine inspect` with the explicit default machine name as the machine-readable discovery interface, update focused fakes and tests, and keep the normative specification independent of command selection

## 2. Documentation

- [x] 2.1 Update Windows prerequisites and the unreleased changelog to describe mode-independent default-machine preparation and retain installation approval guidance; verify no current user-facing documentation requires rootful operation

## 3. Verification

- [x] 3.1 Format changed Go files and verify the change with strict OpenSpec validation, focused unit tests, and the repository unit-test task
- [x] 3.2 Re-run strict OpenSpec validation, focused repository unit tests, and the Windows AMD64 cross-build after the inspection-interface revision
