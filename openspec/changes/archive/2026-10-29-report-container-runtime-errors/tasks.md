## 1. Report Podman's reason for failed commands

- [x] 1.1 Capture a bounded tail of Podman's error output for every Podman command and return it with the failure, keeping the original exit error in the error chain.
- [x] 1.2 Record the deployment failure and Podman's error output in `deployment.log`.
- [x] 1.3 Keep sidecar startup errors free of a repeated cause, with redaction unchanged.
- [x] 1.4 Cover the error message, streamed output, missing output, exit-code queries, and the output bound with unit tests.
