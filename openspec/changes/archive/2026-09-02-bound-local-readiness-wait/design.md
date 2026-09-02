## Context

Local lifecycle operations share generic database polling with cloud workflows, but local startup uses a five-minute timeout and a 2/4/8-second backoff. Local runtime reachability classification already provides actionable guidance and can preserve a causal error. Cloud readiness has a separate operational profile and must remain unchanged.

## Goals / Non-Goals

**Goals:**

- Make unsuccessful local install and start operations finish within 30 seconds.
- Preserve the last database connection error when local reachability guidance applies.
- Keep successful local startup and cloud readiness behavior intact.

**Non-Goals:**

- Change cloud timeouts, backoff policy, or lifecycle command options.
- Add new diagnostics or alter local network-block classification.
- Change runtime startup, endpoint publication, or durability synchronization.

## Decisions

- Give local readiness its own 1/2-second polling values and a 30-second upper bound. The generic polling helper remains shared, while local settings express the materially faster local startup expectation.
- Enforce the local upper bound after the local backend receives its timeout. This covers install, default start, and an explicit `--wait-timeout-minutes` value without narrowing the cloud command's configurable timeout.
- Diagnose an unsuccessful local readiness wait with the existing reachability classifier. Its error wrapper retains the last connection failure and adds guidance only when the runtime reports a network-wide reachability condition. Replacing the connection failure with guidance would hide the actionable underlying cause.

## Risks / Trade-offs

- [Slow but ultimately healthy local environments can exceed 30 seconds] → The bounded wait favors prompt recovery; users can retry after investigating the reported cause.
- [Reachability health can be temporarily ambiguous during container startup] → Keep the existing classifier's conservative checks and only add its guidance when it recognizes the condition.
- [Shared lifecycle code could accidentally alter cloud behavior] → Keep cloud constants and timeout handling unchanged, with regression coverage for local-only policy.

## Migration Plan

No data or configuration migration is needed. A released launcher applies the shorter bound to future local install and start operations; rollback restores the prior launcher behavior.

## Open Questions

None.
