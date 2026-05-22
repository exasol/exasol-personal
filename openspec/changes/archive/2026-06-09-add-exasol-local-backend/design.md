## Context

The launcher currently treats infrastructure through a backend interface that is implemented only by OpenTofu. The existing commands for lifecycle, status, information, SQL connection, and shell access already converge on launcher-owned state plus `deployment.json` and `secrets.json`.

The Exasol Local runner can initialize and start a macOS Virtualization.framework VM, import SSH keys from a shared directory, expose VM services through loopback forwarders, and write `vm-state.json` with runtime ports. The launcher should own the local deployment boundary around that runner instead of exposing the runner as a separate user-facing tool.

```
exasol install local
        |
        v
local backend  --->  mac-runner-aarch64
        |                   |
        |                   v
        |             vm-state.json
        v
deployment.json + secrets.json
        |
        v
info / status / connect / shell
```

## Goals / Non-Goals

**Goals:**

- Provide `exasol install local` for macOS Apple Silicon using the Exasol Local runner.
- Keep existing launcher commands as the user-facing control plane for local deployments.
- Use a launcher-managed share only.
- Use `sys` / `exasol` credentials for the initial local SQL connection.
- Make `destroy` delete the local VM disk/data and launcher-owned runtime artifacts.
- Make `shell container` open a shell inside the Exasol Local database container for local deployments.

**Non-Goals:**

- Generalize local deployments to Linux, Windows, Intel macOS, or other hypervisors.
- Add user-configurable shared folders in the first version.
- Rework the Exasol Local VM image or runner internals beyond the minimum runner contract needed by the launcher.
- Make the Exasol Local installation compatible with the existing Ubuntu remote-exec installation preset.
- Preserve local VM data after `destroy`.

## Decisions

### Add a `local` backend beside `tofu`

The local VM is process and file-system managed, not declarative infrastructure. A dedicated backend keeps local lifecycle behavior explicit while preserving the existing command flow.

Alternative considered: model local deployment as a Tofu preset with local commands. That would force a cloud/IaC abstraction over a local helper process and make lifecycle cleanup less direct.

### Add a local infrastructure preset and Exasol Local installation preset

The infrastructure preset selects `backend: local`. The Exasol Local installation preset is compatible with local deployments and does not require `remote-exec`; the VM image starts the Exasol Local database container during boot.

Alternative considered: reuse the Ubuntu installation preset. That preset requires remote-exec and systemd/COS installation scripts that do not match the Exasol Local VM image.

### Treat the runner as a helper with a narrow contract

The launcher embeds the runner binary at build time, writes it into a launcher-owned runtime directory, and relies on:

- `init` to create VM files.
- `start <cpus> <memory_mb> <data_size_gb>` to start the VM and configure the Exasol Local data disk size.
- `stop` to stop the VM.
- `vm-state.json` to report forwarded SSH, DB, and UI ports.
- The managed shared directory to import `authorized_keys`.

Running the helper from the runtime directory lets the current runner's relative paths remain deployment-local.

Alternative considered: import the runner code into the launcher. That would couple the main launcher binary to macOS Virtualization.framework concerns and complicate cross-platform builds.

### Let the launcher own local artifacts

The backend generates SSH key material, writes the public key into the managed share, reads runner state, and writes normal launcher artifacts. The local `deployment.json` uses loopback addresses and forwarded ports, marks certificate verification as insecure for now, and keeps `shellSupported` enabled.

Alternative considered: have the runner write launcher artifacts directly. That would make the runner aware of launcher state schemas and blur ownership between projects.

### Implement local shell behavior in the backend

`shell host` uses the normal SSH metadata from `deployment.json`. `shell container` is backend-specific for local deployments and opens an interactive shell in the Exasol Local database container instead of running the COS shell script used by cloud deployments.

Alternative considered: provide a separate command for local container shell. Keeping the existing command avoids making users learn a parallel local-only surface.

### Destroy means delete local data

For local deployments, `destroy` stops the VM if needed and removes launcher-owned local runtime files, including VM disk/data and the managed share. This matches the user's intent that local destroy is destructive and complete.

Alternative considered: retain the disk for future restart. That would make `destroy` ambiguous and overlap with `stop`.

## Risks / Trade-offs

- Runner contract drift -> Keep launcher integration behind a small local runner adapter and add contract tests around parsed `vm-state.json`.
- Dynamic forwarded ports change across starts -> Refresh `deployment.json` after every successful `deploy` and `start`.
- Hardcoded local credentials are weak -> Restrict this to the initial Exasol Local path and keep credentials in `secrets.json` so future credential generation can replace the source without changing callers.
- Local TLS fingerprint may be unavailable -> Use `insecureSkipCertValidation` for the local connection until the Exasol Local runtime can expose a stable certificate fingerprint.
- `shell container` depends on the container shell path -> Try `bash` first if available and fall back to `sh`.
- `destroy` data deletion can surprise users -> Command output and docs must make the local destroy semantics explicit.
- macOS-only helper packaging can affect cross-platform builds -> Compile local backend behavior behind runtime platform checks and keep helper assets platform-scoped.

## Migration Plan

Existing deployment directories are unchanged. New local deployments require a new launcher version and a new deployment directory initialized with the `local` infrastructure preset. Rollback is to stop/destroy the local deployment and use an existing cloud preset.

## Open Questions

None for the initial proposal. Future changes can add configurable shares, generated credentials, non-macOS local runtimes, or a richer machine-readable runner interface.
