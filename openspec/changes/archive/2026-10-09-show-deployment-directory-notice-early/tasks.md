## 1. Deliver terminal-output lifecycle

- [x] 1.1 Flush startup queues before command execution; check for launcher updates only after success; flush remaining committed notices and results after completion even on failure, with recovery guidance after the error; emit the install EULA only after full success; and verify streams, JSON, failure, removed-state, documentation, and changelog behavior with `task fmt`, `task all`, and OpenSpec validation.
