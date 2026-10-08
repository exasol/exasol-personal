## Context

See `proposal.md` for motivation. Linux, Windows, and macOS share one Podman installation policy. `exasol stop` removes the Nano container, so every deploy or start runs a new `podman run ... init` against the preserved data directory. The policy treats the data directory as fresh when it has no `exasol.conf`, after migration and incomplete-create recovery, and already adds first-initialization-only arguments in that case. After each successful start the launcher rewrites both `deployment.json` and `secrets.json`; rewriting `secrets.json` was harmless only while the password was constant.

The shipped Nano image accepts `init sys_password_file=<path>` during first initialization. Nano reads the file within moments of starting and logs only a redacted hash. On an initialized runtime, Nano rejects the option whether the file is present, empty, or missing, and the container's `--restart always` policy turns that rejection into a tight crash loop. The image has no shell, and Podman 4.9.3 cannot change a container's restart policy after creation.

The readiness probe deliberately authenticates with invalid credentials, so it never confirms the stored password. The connection debug log includes the driver connection string, and lifecycle commands write debug records to `deployment.log`.

## Goals / Non-Goals

**Goals:**

- Ensure `secrets.json` holds the password Nano was asked to initialize with, from before first initialization until destruction.

**Non-Goals:**

- User-selected initial passwords; the password source stays behind one seam so the option can be added later.
- Changing the readiness probe, cloud credentials, or other launcher state files.

## Decisions

### Persist the password before first initialization

Before the first-initialization container starts, the deploy workflow reuses a generated password already in `secrets.json`, or generates one, writes `secrets.json` atomically, and records a non-secret generated-credential marker in `local/runtime/credential.json`. Artifact writing after start refreshes `deployment.json` and keeps any stored password.

The marker sits beside the Nano data directory, so `destroy` removes it with the data while incomplete-create recovery leaves it in place. Recording it in the launcher state file was rejected because deploy and start write back a copy of that state read before the runtime starts, which would discard the marker. Writing the password after readiness, as the launcher did for the fixed password, was rejected because a crash between initialization and the write would lose the only copy.

### The installation policy requests the password only for a fresh runtime

Freshness is known only after legacy migration, overlay migration, and incomplete-create recovery inside the installation policy. The policy therefore receives a resolver and calls it only for a fresh data directory, so non-fresh starts never handle the password. Deciding freshness in the deploy layer was rejected because macOS legacy data may still be inside the VM before migration.

### A bootstrap directory beside the Nano data directory

The password is written with mode `0600` and no trailing newline to `local/runtime/bootstrap`, or `local/runtime/vm-shared/bootstrap` on macOS, through the execution environment's atomic write, which uses stdin on macOS. Only the first-initialization container mounts the directory read-only at `/run/secrets`.

A single-file mount was rejected because it pins the file, so the container still sees it after host deletion. The Nano data directory was rejected because quarantine moves its content and macOS migration requires it to be empty. A Podman secret was rejected because Podman keeps its own copies and each platform needs extra lifecycle steps.

### The first-initialization container does not restart automatically

The container that receives `sys_password_file` runs with `--restart no`; every later container keeps `--restart always`. Podman replays the original arguments on restart, and Nano rejects the option on an initialized runtime. The exposure is limited to the first container, because `exasol stop` removes it and later starts omit the option. If that database exits before the first stop, status reconciliation reports it stopped and `exasol start` recovers it.

Recreating the container after initialization was rejected because it adds a database restart to every first deployment. Emptying or deleting the file was rejected because Nano rejects the option regardless of the file, producing the same crash loop. A wrapper entrypoint and `podman update --restart` are unavailable. If Nano ignores the option on an initialized runtime, the first container can use the usual restart policy.

### Bootstrap material is removed on every outcome

Cleanup empties the bootstrap directory, including temporary files an interrupted write leaves beside the password file, and keeps the directory because a running container may mount it. It runs before every local deploy or start, after every attempt, and from a signal handler on SIGINT or SIGTERM, because the launcher exits right after its signal handlers run and would otherwise skip cleanup at the end of the start. A failed first deploy stops the container before cleanup so a retry starts from a known state, and `destroy` removes `local/`.

Stopping the container from the signal handler was rejected because on macOS it means stopping a VM while the process is exiting; a container left running stays unverified, so the next start checks its password.

### The stored password is verified until a login succeeds

The generated-credential marker also records whether a login with the stored password has succeeded. While it has not, every local deploy or start opens one session as `sys` with the stored password, bounded by its own timeout, after readiness, and fails with recovery guidance and stops the container if the password is rejected. A successful login marks the credential verified, after which starts skip the check because users may change the `sys` password through SQL.

Verifying only the start that initialized the database was rejected because a retry after a rejected or interrupted check would find an initialized runtime, skip the check, and record a running deployment whose stored password the database does not accept. The marker is written atomically so an interrupted write cannot block later starts.

### Existing deployments keep their credentials

A non-fresh start never generates a password. When `secrets.json` is missing, a deployment without the generated-credential marker receives `exasol`, the password it was initialized with, while a deployment with the marker fails with restore-or-redeploy guidance instead of receiving an invented credential. A fresh initialization replaces a stored `exasol`, because a fresh runtime has no password yet.

### Passwords and logs

Passwords are 16 characters from `[A-Za-z0-9]` generated with `crypto/rand`; the alphabet needs no escaping in connection strings, shells, or Nano's file parsing. The connection debug log records user, host, port, and certificate validation mode, and the password is held in a type whose string and log forms are redacted.

## Risks / Trade-offs

- [Windows and macOS mounts differ from Linux] → The bootstrap directory uses the same mount route and atomic write as the Nano data directory on each platform.
- [A crash after persisting but before initialization] → The retry reuses the stored password.
- [A crash or kill before bootstrap cleanup] → Every local start empties the bootstrap directory, and `destroy` removes `local/`.
- [The first-initialization database does not restart automatically] → It is reported stopped, and `exasol start` recovers it.
