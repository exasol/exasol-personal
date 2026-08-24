## Context

The local runtime interface owns host and container shell execution as well as database-endpoint health checks. The deployment backend currently sends every shell error through the database reachability classifier, so an unhealthy endpoint can replace a runner failure or an unsupported-operation error. The macOS runtime already invokes shells without SSH, while the reachability specification still describes guest IPs, SSH ports, and SSH connection failures.

The implementation spans the runtime and deployment packages, and the existing shell fixture does not protect the interactive process contract. See `proposal.md` for motivation and the delta specs for observable requirements.

## Goals / Non-Goals

**Goals:**

- Report the actual shell failure instead of unrelated connectivity guidance.
- Provide actionable guidance when a local connectivity problem is recognized without hiding the underlying operation failure.
- Keep permanent requirements focused on behavior observable from the built CLI.
- Protect the runner process boundary and related deployment contracts with focused regression tests.

**Non-Goals:**

- Reintroducing SSH or exposing guest-network details.
- Changing cloud shell behavior, command-line syntax, or the deployment JSON schema.
- Changing which supported local-deployment platforms provide shell access.
- Rewriting unrelated architecture-oriented capabilities as part of this change.
- Rewriting historical commits or reverting unrelated formatting-only changes.

## Decisions

### Specify behavior at the CLI boundary

Permanent requirements will describe failures, guidance, diagnostics, and shell behavior that can be verified against the built CLI. Runtime delegation, endpoint-health classification, process arguments, working directories, standard streams, and error identities remain in this design, implementation tasks, and unit tests because they are internal contracts that may change without changing the product behavior.

The shell behavior touched by this change will be specified in `exasol-local-deployment` through user-visible supported and unsupported behavior. The reachability requirements will be consolidated around accurate errors and corrective guidance instead of enumerating commands, endpoints, and classification states. Unrelated requirements already present in architecture-oriented capabilities are outside this change.

### Return runtime shell errors without database reachability classification

The local backend will return the result of each runtime shell method directly. Reachability classification remains appropriate for database startup and connection failures, but a database endpoint cannot explain a failure to launch a runtime-owned shell. This applies to every shell error, not only known unsupported sentinels, so runner exit failures and future runtime-specific errors cannot be masked.

An alternative was to bypass classification only for known unsupported errors. That would leave macOS runner failures vulnerable to the same misclassification and require the deployment layer to know every runtime error category.

### Add platform context while preserving unsupported-error identity

The Linux runtime will wrap the existing host-shell and container-shell unsupported sentinels with Linux host-runtime context. This satisfies the existing platform-specific user-facing contract while preserving machine-detectable error identity for callers and tests.

An alternative was to create new Linux-only sentinels. That would split the common runtime contract and make callers handle platform-specific error types unnecessarily.

### Define reachability in terms of application endpoints

Reachability classification will consume the runtime's reported application-port health and remain conservative: it will classify a reachability problem only when every reported application endpoint is blocked or timed out. Reachable, refused, missing, unknown, or unavailable health preserves the operation's original error. The diagnostics command will report runtime status, bound application ports, their health, and SQL readiness without depending on guest IP or transport-port metadata.

Keeping compatibility fields solely to support the retired SSH-oriented diagnostics was rejected because new runner state and deployment metadata intentionally omit those fields.

### Test the interactive runner contract at process boundaries

Executable runner fixtures will record arguments without flattening them, verify the runtime working directory, consume known standard input, write distinct standard output and error, and support a controlled non-zero exit. Host and container cases will be independently named so failures identify the broken contract. Deployment tests will assert unsupported errors by identity as well as message context.

Focused tests will also cover the already-required shell-support JSON field, both branches of conditional connection-instruction rendering, and repair of an otherwise unchanged staged artifact whose permission mode is wrong. Obsolete fake-runner branches will be removed when no production path exercises them.

## Risks / Trade-offs

- **Risk:** Users may see an operation-specific shell error where they previously saw reachability guidance. → **Mitigation:** This is intentional because connectivity guidance cannot explain a failure to open a shell; the reported error retains useful context.
- **Risk:** Script-based tests can be sensitive to shell behavior. → **Mitigation:** Keep fixtures POSIX-only behind the repository's existing platform guard and use files/streams for exact observations instead of shell word flattening.
- **Risk:** Tight assertions can over-specify incidental command construction. → **Mitigation:** Assert the documented runtime boundary—argument boundaries, working directory, streams, and error propagation—without asserting unrelated environment or process details.

## Migration Plan

No persisted-data migration is required. Implement the error-routing and Linux wrapping changes, update diagnostics, fixtures, and the changelog, then run formatting, linting, and the unit-test suite. Rollback consists of reverting the code and spec changes; deployment artifacts remain compatible in either direction.
