The implementation task below maps to exactly one code-only commit that passes
the full check suite. Group 2 is validation and produces no commit.

## 1. Stop OpenTofu on interrupt

- [x] 1.1 Run OpenTofu so that the launcher alone receives terminal
  interrupts, interrupt it once on SIGINT or SIGTERM, keep the interrupted
  operation from recording a failure, and exit after OpenTofu has exited.
  Verify `task tests-unit` covers interrupting, waiting, and a command started
  after an interrupt. Verify `task tests-launcher` covers deploy and destroy
  interrupted with SIGINT and SIGTERM, delivered to the process group and to
  the launcher only, checking that OpenTofu stopped, the persisted
  `interruptedDuringOperation`, and the `status` and `info` output. Add the
  changelog entry.

## 2. Validation

- [x] 2.1 Run `task all` and confirm formatting, linting, unit tests and the
  launcher tests pass on the finished branch.
