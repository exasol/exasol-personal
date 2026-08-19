## 1. Host Runtime Framework

- [x] 1.1 Generalize the Linux host runtime into a shared host runtime with an injected host runtime environment preparer
- [x] 1.2 Add pre-workflow backend preparation and thread deploy/start runtime preparation options through the interfaces
- [x] 1.3 Add focused tests proving preparation ordering, state preservation on failure, and unchanged Linux behavior
- [x] 1.4 Run formatting, focused tests, and full lint, then commit the framework change

## 2. Windows Host Environment

- [x] 2.1 Implement testable Windows command execution, registered PATH refresh, winget installation, and Podman verification
- [x] 2.2 Implement default Podman machine discovery, rootful initialization, start, and approved rootless conversion
- [x] 2.3 Add command-layer host-change prompts and `--auto-approve` for install, deploy, and start
- [x] 2.4 Select the shared host runtime with Windows preparation on Windows AMD64 and reject unsupported Windows architectures
- [x] 2.5 Cover approval, retry, command failure, platform selection, configuration, shell, endpoint, and health behavior with focused tests
- [x] 2.6 Run formatting, focused tests, Windows cross-build, and full lint, then commit the Windows runtime change

## 3. Windows Lifecycle Integration

- [x] 3.1 Cover built-in Podman setup, SQL readiness, stop/start persistence, and destroy cleanup in the regular Windows integration suite
- [x] 3.2 Add failure diagnostics and keep the install/PATH regression checks in one integration-test process
- [x] 3.3 Run workflow-oriented checks and full lint, then commit the CI change

## 4. Documentation

- [x] 4.1 Document Windows AMD64 local prerequisites, approval behavior, lifecycle capabilities, and limitations in the README and changelog
- [x] 4.2 Update architecture and CI documentation without duplicating operational instructions
- [x] 4.3 Run full lint and commit the documentation change

## 5. Final Verification and Archive

- [x] 5.1 Run strict OpenSpec validation, `task all`, Windows cross-build, and final repository checks
- [x] 5.2 Archive the completed OpenSpec change and validate the archived specifications
- [x] 5.3 Run full lint and commit the OpenSpec archive
