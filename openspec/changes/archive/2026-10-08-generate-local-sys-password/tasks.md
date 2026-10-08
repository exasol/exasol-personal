## 1. Redact Connection Logging

- [x] 1.1 Log database connections by user, host, port, and certificate validation mode instead of the connection string, and hold the password in a type whose string and log forms are redacted. Add unit tests that capture debug logging and formatting with a marker password, extend the live connect leak test to `--log-level debug`, add the changelog entry, and run `task all`. Commit as `fix: Redact database credentials from debug logs`.

## 2. Persist Secrets Atomically

- [x] 2.1 Write `secrets.json` through a same-directory temporary file with mode `0600`, synced and renamed into place. Add unit tests for round-tripping, file mode, and a failed write preserving the previous file, and run `task all`. Commit as `fix: Write secrets.json atomically`.

## 3. Preserve Stored Local Secrets

- [x] 3.1 Refresh `deployment.json` on every local deploy and start, but create `secrets.json` only when it is missing. Add tests proving a stored password survives deploy and start and that `deployment.json` still follows port changes, then run `task all`. Commit as `refactor: Stop rewriting local secrets on every start`.

## 4. Generate the First-Initialization Password

- [x] 4.1 Add the bootstrap directory beside the Nano data directory and give the installation policy an initial-password resolver that it calls only for a fresh data directory; the policy writes the bootstrap file, mounts the directory read-only at `/run/secrets`, passes `sys_password_file`, runs that container with `--restart no`, and removes the file at every start and after a failed run. Expose host-side bootstrap removal from each local runtime. Add unit tests for freshness, argv content, restart policy, mounts, and cleanup paths, and run `task fmt` and `task lint`. Commit as `feat: Support an initial sys password in local Podman installations`.
- [x] 4.2 Add the 16-character password generator, resolve the credential in the deploy workflow before first initialization, record that the credential was generated, remove bootstrap material after readiness and on every first-initialization failure or interrupt, and handle missing secrets for legacy and generated credentials. Update the user documentation and the changelog, and add unit tests for retry and recovery reuse and legacy deployments, plus live local tests for generated credentials, connect, leaks, cleanup, and stop and start. Run `task fmt` and `task lint`. Commit as `feat: Generate the initial sys password for fresh local deployments`.

## 5. Verify the Generated Credential

- [x] 5.1 After readiness on a first initialization, authenticate once as `sys` with the stored password, and on rejection stop the container and fail with recovery guidance. Add tests for acceptance, rejection, non-authentication failures, and later starts skipping the check, and run `task fmt` and `task lint`. Commit as `feat: Verify the generated credential after first initialization`.

## 6. Verify Other Platforms

- [x] 6.1 Run `task all` and the local deployment tests on Linux, then confirm the bootstrap mount and run the local deployment tests on Windows and macOS.
