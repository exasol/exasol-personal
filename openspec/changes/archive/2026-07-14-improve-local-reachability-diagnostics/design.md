## Context

The local preset runs the Exasol database inside a macOS VM managed by an embedded runner binary (`internal/localruntime`), which the launcher invokes as a subprocess and communicates with via an embedded runner CLI and a small Unix-domain-socket status protocol (`{"request":"status"}` → `{"status":"running"}`). The runner forwards VM guest ports to `127.0.0.1` on the host.

Investigation established that the launcher's own dial to `127.0.0.1:<port>` almost always succeeds regardless of macOS's Local Network permission state, because the forwarding listener is bound independently of guest reachability. The actual TCC-gated hop happens one level down, inside the runner's own connection to the VM's guest IP — a hop the launcher cannot observe directly today. This means the launcher cannot classify this failure from its own dial behavior alone; it needs the runner to tell it.

Two existing gaps compound the problem:
- `internal/deploy/local_backend.go`'s `Start` ignores its `waitTimeoutSeconds` parameter entirely (`_ int`), unlike the tofu-backed cloud path which applies it via `context.WithTimeout`. `Deploy` (used by `exasol install local`) has no timeout parameter in its interface signature at all (`DeployOptions` only carries an unrelated dependency-lockfile flag). Either way, a blocked local deployment polls forever rather than ever failing.
- `internal/deploy/ready.go`'s `verifyDatabaseConnection` treats any non-SQLSTATE-08004 error as "not ready yet," so a permanently broken network path looks identical to a database that's still booting.
- `exasol connect`'s dial (`internal/connect/exasol/database.go`) never receives `ctx` at all, so it has no timeout of its own either.

## Goals / Non-Goals

**Goals:**
- Distinguish "local runtime reachability problem" from "generic database/readiness failure" in the error shown for install/start, `exasol connect`, and `exasol shell host`/`container`.
- Do this using only an external, versioned-together contract with the runner (a new status-socket request), not by inferring from the launcher's own loopback dial, which we've established is not a reliable signal.
- Bound every local wait that currently has none.
- Add a read-only `exasol diag local` surface usable independent of any failure.

**Non-Goals:**
- Redesigning the runner's VM/networking architecture (NAT, port forwarding mechanism) — out of scope for this change entirely.
- Detecting or naming the specific invoking application (iTerm2, kitty, VS Code, etc.) via process-tree introspection. A generic, always-correct explanation is preferred over a sharper but fragile one.
- A generic multi-backend `exasol diag <preset>` framework. This change adds `exasol diag local` specifically; extending diagnostics to cloud backends is future work.
- Automated end-to-end reproduction of the real macOS permission block (see Risks).
- Re-classifying reachability mid-poll: the fast-path check runs once, before the readiness backoff loop begins. A permission change that occurs after the wait has already started (e.g. granted partway through) is not detected until the next invocation.

## Decisions

### The runner's status socket gains a new, separate `health-check` request, not an extension of `status`

`status` is called from routine, frequently-hit code paths (e.g. `reconcileLocalVMState`) purely to check whether the VM process is alive; it must stay cheap and side-effect-free. Reachability classification requires the runner to actively dial the VM guest network, which can be slow if a permission block manifests as a hang rather than an immediate error (unconfirmed — see Open Questions). Folding that into `status` would silently make every routine status check occasionally slow. Instead, this change assumes a new, distinct request (contract only, no runner internals):

```
Request:  {"request": "health-check"}
Response: {
  "ports": {
    "ssh": {"state": "reachable" | "refused" | "blocked" | "timeout"},
    "db":  {"state": "reachable" | "refused" | "blocked" | "timeout"}
  }
}
```

- `health-check` probes every forwarded port in one shot, not just the one the caller cares about — required for differential diagnosis (below). It runs only when explicitly requested; nothing in this change introduces periodic/background polling of the VM.
- The launcher only consumes this classified `state` field. It does not know, and must not encode any assumption about, how the runner arrived at it (dial errno, timing heuristic, etc.) — that reasoning lives entirely on the runner side.
- If the request is unsupported or the response fails to decode, the launcher falls back to today's generic failure path rather than erroring on unexpected shape. Because the runner is embedded and staged fresh with each launcher build (see `assets/localruntimebin`), version mismatch should only arise from an already-running runner daemon from before an upgrade — a real but narrow case, not a long-term compatibility surface to design around.

**Alternatives considered**: Inferring reachability purely from the launcher's own loopback dial (rejected — shown not to reliably surface the real failure, since that dial usually succeeds regardless of the actual block). Encoding the raw OS error (errno/string) in the response and classifying it in the launcher (rejected — ties the launcher to OS/error-text specifics that belong on the runner side, and we've confirmed the SQL driver's own error already discards the underlying cause in at least one path, so the launcher would be no better positioned than the runner to do this classification anyway).

### Differential diagnosis across ports is the primary classification signal

A single port's failure is ambiguous: it could mean "database still starting," "database crashed," or "network path blocked." Comparing across ports resolves this:

| SSH | DB | Diagnosis |
|---|---|---|
| reachable | reachable | healthy |
| reachable | refused/timeout | database-specific problem (still booting, or crashed) |
| blocked/timeout | blocked/timeout | network-wide reachability problem (the case this change targets) |
| blocked | reachable | (should not occur; treat as reachability problem defensively) |

Because normal operation never naturally generates SSH traffic during `start`/`connect` (only `shell host`/`container` do), this comparison is only possible because `health-check` deliberately probes every port on demand, not just the one in use.

### New typed error, not reliance on unwrapping the SQL driver's error

`exasol-driver-go`'s `NewConnectionFailedError` (the source of the `W-EGOD-14` error text seen during exploration) constructs its `DriverErr` with `cause: nil`, discarding the underlying `*net.OpError`/errno entirely. `errors.Is`/`errors.As` against that error can never recover the original cause. This change introduces a distinct error type (mirroring the existing `blockedStateError` pattern in `internal/deploy/shared.go`: `Error()` returns the complete, user-facing, actionable message; `Unwrap()` exposes a sentinel for `errors.Is` elsewhere) built directly from the `health-check` classification, independent of whatever the SQL driver or SSH client separately report.

### Message content is generic, not app-detected

The error names example invoking environments (iTerm2, kitty, VS Code, a sandboxed agent host) and explains that Local Network permission applies to the invoking terminal/editor/agent even though the target is `127.0.0.1`, plus how to grant it. Real process-tree detection was considered and rejected: it can be wrong, adds platform-fragile logic, and a generic explanation is always correct.

### `exasol diag local`, not a generic multi-backend diagnostics command

Local's failure modes are specific enough (platform/arch support, VM/runner lifecycle, port forwarding) that a local-specific command carries more useful information than a generic one would. It reports: VM status, guest IP, bound host ports, per-port `health-check` state, SQL-level readiness, and whether the current OS/architecture is supported at all (today, mac/arm64 only — a check that exists nowhere as user-facing diagnostics currently).

### Scope: install/start, connect, shell host/container — not stop/destroy/status

Investigation confirmed `stop`/`destroy`/`status` only invoke the runner's CLI/process-level control path and never dial a forwarded port, so they cannot exhibit this failure mode. No changes are needed there; this is a deliberate scope decision, not an oversight.

### `Start`/`Stop` recovery reuses `Destroy`'s existing pattern

`Destroy` already transitions a deployment to `WorkflowStateInterrupted` when its backend call fails, via `markDestroyInterrupted`. `Start` and `Stop` lacked the equivalent: a backend failure left the deployment in `WorkflowStateOperationInProgress` with no path back to a resolved state. Rather than duplicating `markDestroyInterrupted`'s logic a second and third time, this change generalizes it into a single `markOperationInterrupted` helper parameterized by operation name, and has `Destroy` delegate to it as well. This keeps all three lifecycle operations' failure-recovery behavior identical and defined in one place. This part of the change is backend-agnostic (it lives in the shared lifecycle-control code, not the local backend), unlike the rest of this proposal.

## Risks / Trade-offs

- **[Risk]** It is unconfirmed whether a genuine macOS Local Network permission denial produces a synchronous error or a silent hang on the runner's guest-IP dial, for privacy-motivated reasons Apple applies to other TCC gates. → **Mitigation**: the runner-side classification (out of scope for this proposal, but this proposal's contract must tolerate either) should treat persistence past a generous grace period as sufficient for a `blocked`/`timeout` verdict, not depend on catching a specific errno. This proposal's launcher-side logic is agnostic to which of `blocked`/`timeout` it receives — both map to the same reachability error.
- **[Risk]** A genuine macOS permission denial cannot be exercised in automated CI. → **Mitigation**: the classification logic can be verified against synthetic runner responses; a real permission denial can be manually approximated with an OS-level sandbox that denies outbound network access while permitting loopback and local IPC, without needing an actual TCC grant/deny cycle.
- **[Risk]** An already-running runner daemon from before an upgrade won't understand `health-check`. → **Mitigation**: covered by the generic-fallback behavior above; existing deployments are never forced to restart or lose VM state to benefit from this, they just don't get the sharper error until the runner is next restarted (which happens on `start`, without destroying data).
- **[Risk, resolved]** The runner's own pre-existing internal SSH-readiness gate (used to confirm VM boot before reporting success) used to fail and return *before* the runner created any port forwarders, and used to tear down the VM and exit the daemon process entirely on that failure. If the network was blocked from the very start of a `start`/`install` run, `classifyLocalReachability` had no forwarder data left to classify against — so this one scenario surfaced the runner's raw error text instead of the polished reachability message. This shared a root cause with a separate bug: the VM-teardown call on that path could block indefinitely, leaving an orphaned daemon holding the VM disk and breaking the *next* `start` attempt with an unrelated storage conflict. → **Resolved** (runner-side change, see the reference proposal for exasol-local-vm): port forwarders are now created before the SSH-readiness gate runs, and a failure there no longer tears the VM down — the daemon stays alive and queryable so `health-check`/`diag local` can report real per-port reachability. The VM-teardown call used elsewhere is now bounded by a timeout so it can no longer block indefinitely.

## Migration Plan

No data migration. Purely additive behavior change: existing successful local deployments are unaffected (the localhost connection contract is unchanged); failures gain a more specific error message and, for install/start, an enforced timeout where none existed before. No flag or opt-in — this replaces the existing generic failure path outright once shipped, since the old behavior (indefinite hang, misleading generic message) has no one relying on it as correct.

## Open Questions

- Whether real TCC Local Network denial yields a synchronous error or a hang on the runner's own dial is unconfirmed (requires a GUI session to grant/deny the permission for a real invoking app). Recommended to settle before or shortly after implementation, but the design above does not depend on the answer.
