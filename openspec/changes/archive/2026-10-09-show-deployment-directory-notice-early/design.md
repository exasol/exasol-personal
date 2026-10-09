## Context

`rootCmd.PersistentPreRunE` resolves deployment context and queues the selected-directory notice. `Execute` currently flushes terminal messages only after Cobra returns, while the automatic update check runs in pre-run. `install` queues its EULA in `PreRunE`, before deployment in `PersistentPostRunE`. This makes an interactive `connect` notice late, checks for updates even when a command later fails, and can show an EULA for an unsuccessful install.

## Goals / Non-Goals

**Goals:** Make message delivery follow command phases for every command, keep stream and JSON contracts, and show final update guidance and EULA only after relevant success.

**Non-Goals:** Change directory precedence, version-check throttling or network protocol, primary command results, or error-specific recovery guidance.

## Decisions

1. After shared root startup resolves deployment context, call the same generic terminal-queue writer used at completion. It drains notices, outputs, and calls to action queued so far. Currently the selected-directory notice is its only queued message. Every visible, non-empty call-to-action batch begins with one blank line, without tracking whether an earlier flush printed anything.
2. Record the command only after its shared pre-run succeeds. After Cobra returns, let the update-hint helper reject failed, ineligible, or absent commands before any check; then close deployment logging and flush all remaining messages on success. This excludes help, parse failures, and command failures; `version` stays exempt.
3. Before a post-success check, verify the selected deployment still has launcher state. If `remove` or `destroy --remove` deleted it, skip without recreating the directory. A missing state file on another successful command is likewise a no-op; stat errors are best-effort debug diagnostics.
4. Leave fresh `init` EULA at successful initialization. Move `install` EULA queuing to the end of its successful persistent post-run, after deployment and connection instructions have been prepared. A failed install therefore has no EULA.

## Risks / Trade-offs

- A startup notice may remain visible when later compatibility or command validation fails. It correctly identifies the selected directory; invalid flag groups still fail before resolution.
- Moving update checks after command completion may expose a newly initialized deployment to its first automatic check, subject to the existing persisted setting and throttle. Tests verify success/failure and deleted-state behavior.
- Separate stdout and stderr streams do not guarantee relative ordering for downstream consumers that capture them separately. In a combined terminal, calls to action remain visually last.
