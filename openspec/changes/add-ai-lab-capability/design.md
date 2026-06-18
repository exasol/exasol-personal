## Context

Exasol Personal provisions a single compute instance (on AWS: an Ubuntu 22.04 `r6i.xlarge`) that runs the Exasol database inside a rootless container managed by **C4** (`c4 host play`, `CCC_PLAY_ROOTLESS=true`). The host installs only a small package set (`wget curl tar net-tools jq uidmap`); **Podman is not installed by default**, and C4 manages the database container through its own bundled runtime.

The Exasol **AI Lab** ships as an official container image (`exasol/ai-lab`) running JupyterLab on port **49494** as the non-root user `jupyter`, with notebooks/config under `/home/jupyter/notebooks`. It connects to an *external* Exasol database (our case — the DB is already on the box). Its connection settings live in a **Secure Configuration Storage (SCS)** — an encrypted SQLCipher file accessed via the `exasol-notebook-connector` Python library (`exasol.nb_connector.secret_store.Secrets` + the `AILabConfig` key enum), and unlocked with a master password.

This change adds AI Lab as an opt-in, pre-wired companion to the database. It deliberately follows the existing **`admin-ui-access`** capability pattern: presets declare support via `compatibility.provides`, the backend writes optional connection metadata to `deployment.json`, connection instructions render conditionally, and the local backend omits it.

## Goals / Non-Goals

**Goals:**
- One-command, zero-config AI Lab co-located with the database on supporting cloud infrastructure.
- Pre-seed DB + BucketFS connection so notebooks work immediately with no manual setup.
- Reachable via a URL (exposed port) protected by a password and the existing address allow-list.
- Reproducible behavior consistent with how the launcher already exposes the Admin UI.

**Non-Goals:**
- AI Lab on local deployments (deferred — UDFs and BucketFS are unavailable locally; the capability is shaped so local can opt in later with no consumer changes).
- AI Lab's integrated Docker-DB and Script-Language-Container build features (they require docker-socket/privileged access; out of scope — we always use the external DB).
- Multi-user / JupyterHub deployment.
- Pinning/reproducing a specific AI Lab image version (decision: track latest).

## Decisions

### Install trigger: both a flag and a standalone command
`exasol install <preset> --with-ai-lab` covers the greenfield path; `exasol ai-lab install` adds AI Lab to an already-running deployment without redeploying the database. Rationale: users decide they want AI Lab both up front and after the fact, and decoupling the second path avoids forcing a DB redeploy. Alternative (default-on) was rejected — every deployment would pay the shared-instance resource cost and gain an exposed service unconditionally.

### Container runtime: install Podman on the host
Since Podman is not present, the installation assets add the `podman` package (rootless prerequisites like `uidmap` already exist). We run AI Lab as a **separate rootless Podman container**, independent of C4's database container. Alternative (reuse C4) was rejected: C4 is purpose-built for the database cluster, not arbitrary side containers.

### Installation mechanism: shared OS-preset script + per-cloud hook registration
The AI Lab install **script lives in the installation (OS) preset** (`assets/installation/ubuntu/`, alongside `installExasol.sh`/`runInfraHookScripts.sh`) so it is cloud-agnostic and written once. Each **infrastructure preset** opts in by registering that script in its `postInstall.scripts` hook (run on n11 after the database is ready, via `runInfraHookScripts.sh`) and injecting the AI Lab settings/secrets through its own `cloudinit.tf`. The script pulls the image, sets the Jupyter password, seeds the SCS, and installs a Podman Quadlet/systemd unit with `loginctl enable-linger` so the container survives reboots and `stop`/`start`. Rationale: keeps the OS-level mechanics in one place and reuses the established, declarative extension point rather than inventing new install plumbing.

### Connectivity: container → database/BucketFS on the same host
AI Lab reaches the DB at the host over Podman's `host.containers.internal` (fallback: the node's private address), DB port `8563`, BucketFS port `2581`. The launcher seeds the SCS keys via the notebook-connector library: `db_host_name`, `db_port`, `db_user`, `db_password`, `db_encryption=True`, `cert_vld=False` (Exasol Personal uses a self-signed cert), `storage_backend=onprem`, `use_itde=False`, plus the `bfs_*` BucketFS keys.

### Secrets: auto-generate, store in `secrets.json`, reference (don't print) in instructions
The SCS master password and Jupyter password are generated as OpenTofu `random_password` outputs and written into `secrets.json` (mirroring `dbPassword`/`adminUiPassword`), with matching fields added to `config.Secrets`. Connection instructions show the AI Lab **URL** and point the user to `secrets.json` for the passwords — they are **not** echoed to the terminal, exactly as the DB and Admin UI passwords are handled today (`connection_instructions.go` prints `Password: <stored in …/secrets.json>`). This achieves "no manual config": the user never has to invent or type connection details — the pre-seeded SCS carries them, and the master/Jupyter passwords are waiting in `secrets.json`.

### Network exposure: dedicated security-group ingress, gated by `allowed_cidr`
A new ingress rule opens the AI Lab port (subject to the existing `allowed_cidr`), alongside the Jupyter password gate. The ingress rule is provider-specific (AWS security group ≠ Azure NSG ≠ Exoscale/STACKIT), so it lives in each infrastructure preset; there is no shared "open a port" abstraction. Mirrors how 8443/8563 are exposed today. Alternative (SSH-tunnel-only) was rejected per product intent ("expose the port so the user can connect"), but the tunnel remains available for users who lock `allowed_cidr` down.

### Capability shape: new `ai-lab-access`, modeled on `admin-ui-access`
Optional `deployment.json` metadata object, conditional connection instructions, local backend omits it. Keeps the launcher's "infrastructure provides X → metadata → instructions" pattern consistent.

## Risks / Trade-offs

- **Resource contention on the shared instance** → AI Lab recommends ~2 cores / 8 GiB; the DB is the heavy tenant on a 4 vCPU / 32 GiB `r6i.xlarge`. Mitigation: opt-in only; document bumping `--instance-type` for heavier ML work.
- **Larger attack surface (exposed port + two stored secrets)** → Mitigation: gate behind `allowed_cidr`, require the Jupyter password, store secrets with the existing secrets model; document tunnel-only as the hardened option.
- **Tracking `latest` makes deployments non-reproducible / an upstream change could break installs** → Accepted per decision; mitigation: surface the resolved image digest/version in diagnostics and keep the image reference overridable.
- **Installing Podman outside C4's control could perturb the rootless setup** → Mitigation: run AI Lab in a distinct rootless tree; validate on a live deployment that it does not interfere with the database container.
- **Master password stored at rest** → Accepted trade-off for zero-config; same posture as the existing DB password in the deployment's secrets.

## Migration Plan

Additive and opt-in: existing deployments are unaffected until a user passes `--with-ai-lab` or runs `exasol ai-lab install`. Rollback = remove the AI Lab container/unit and the security-group rule; the database deployment is untouched. No `deployment.json` migration needed (the AI Lab object is optional and absent for existing deployments).

## Open Questions

- Exact reachable DB/BucketFS endpoint from a rootless Podman container on the host (`host.containers.internal` vs private IP; whether 8563/2581 are bound on a container-reachable address) — to confirm on a live deployment.
- The SCS file name/location the current AI Lab notebooks expect by default, so the pre-seeded file is picked up without notebook edits.
- Whether `exasol ai-lab install` should also have an uninstall/teardown counterpart in this change or a follow-up.
- Generalization to other cloud presets (Azure/Exoscale/STACKIT): AWS is implemented now; others opt in by adding `ai-lab` to `compatibility.provides` plus their own ingress rule.
