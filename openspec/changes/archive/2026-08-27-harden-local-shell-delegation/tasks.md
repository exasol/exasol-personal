## 1. Preserve Runtime Shell Errors

- [x] 1.1 Return host-shell and container-shell runtime errors directly from the local deployment backend, and verify deployment tests show that blocked health for every reported application endpoint cannot replace either error with a reachability error.
- [x] 1.2 Wrap Linux host-runtime shell failures with platform-specific context while preserving the host-shell and container-shell unsupported error identities, and verify runtime and backend tests assert both `errors.Is` identity and user-facing Linux context.

## 2. Protect the Interactive Runner Boundary

- [x] 2.1 Replace the flattened host-shell runner fixture with a Given/When/Then test that records argument boundaries and working directory, consumes known standard input, and emits distinct standard output and error; verify `go test -race ./internal/localruntime -run TestMacVMRuntimeOpenHostShellDelegatesToRunner` passes.
- [x] 2.2 Add `TestMacVMRuntimeOpenContainerShellDelegatesToRunner` to verify the complete container-shell argument vector and standard streams without sharing assertions that could hide a host/container regression; verify the targeted test passes.
- [x] 2.3 Add a controlled non-zero runner exit case and verify the runtime returns a contextual error that preserves the underlying command failure.

## 3. Fill Contract-Coverage Gaps

- [x] 3.1 Extend runner-state endpoint tests to assert shell support and raw deployment JSON tests to assert `connection.shellSupported` is `true` while SSH fields remain absent; verify the targeted local-runtime and deployment artifact tests pass.
- [x] 3.2 Cover both branches of conditional connection-instruction rendering by asserting local output omits the SSH alternative and an SSH-capable deployment still renders it; verify the targeted rendering tests pass.
- [x] 3.3 Add a regression test where a staged runtime artifact has unchanged content metadata but the wrong file mode, and verify materialization repairs the mode without requiring a content change.
- [x] 3.4 Update reachability comments and fixtures to use application-endpoint terminology and remove the unused fake-runner `stop` behavior; verify the local reachability and diagnostics tests pass.

## 4. Document and Verify

- [x] 4.1 Add an `Unreleased` `Fixed` changelog entry for preserved local shell errors, then run `env GOFLAGS=-tags=containers_image_openpgp GOSUMDB=sum.golang.org GOTOOLCHAIN=auto task fmt lint tests-unit` and verify formatting, linting, and the complete unit-test suite succeed.
