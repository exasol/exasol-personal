## 1. Installation (OS) preset — shared across all clouds

- [x] 1.1 Install the `podman` package on the host during preparation (in the `ubuntu` installation preset)
- [x] 1.2 Add a post-install hook script (alongside the existing shared scripts, e.g. `installExasol.sh`/`runInfraHookScripts.sh`) that pulls the latest `exasol/ai-lab` image and runs it via Podman, publishing the AI Lab port and mounting a persistent notebooks volume
- [x] 1.3 Set the Jupyter password on the container from the value injected by the infrastructure preset
- [x] 1.4 Generate/seed the SCS: write the master password and the database connection parameters (host, port, user, password, encryption, `cert_vld=false`, `storage_backend=onprem`, `use_itde=false`) and the BucketFS parameters (`bfs_*`) using the notebook-connector configuration keys
- [x] 1.5 Confirm the seeded SCS file name/location matches what the AI Lab notebooks load by default, and that connection works against the self-signed DB certificate — VERIFIED on AWS: SCS at `/home/jupyter/notebooks/ai_lab_secure_configuration_storage.sqlite` (owned by `jupyter`), `open_pyexasol_connection` returned `SELECT 1 → [(1,)]`, version `2026.1.0` with `cert_vld=false`. SCS filename updated from `ai_lab_config.db` to match `notebook-connector` `DEFAULT_FILE_NAME`. Verified compatible with `exasol/ai-lab:6.0.0` / `notebook-connector 3.0.0` (2026-06-24): no path or key changes affect our seeding.
- [x] 1.6 Install a Podman Quadlet/systemd unit and enable user lingering so AI Lab restarts after reboot and survives deployment stop/start
- [x] 1.7 Verify the AI Lab container does not interfere with the C4-managed database container (rootless isolation) — VERIFIED on AWS: AI Lab container runs rootless alongside the C4 DB container; DB stays queryable while AI Lab is up
- [x] 1.8 Enablement flag (default off): realized as an **infrastructure** variable `with_ai_lab` (must be known at Terraform plan time) and surfaced to the host via `infrastructure.json` `aiLab.enabled`, rather than an installation-preset variable

## 2. Infrastructure preset opt-in (AWS first)

- [x] 2.1 Declare `ai-lab` in the AWS preset's `compatibility.provides`
- [x] 2.2 Add an infrastructure-preset variable for the AI Lab port (default 49494), plus the `with_ai_lab` enablement variable
- [x] 2.3 Add a security-group ingress rule for the AI Lab port, gated by `allowed_cidr` (conditional on `with_ai_lab`)
- [x] 2.4 Generate the SCS master password and Jupyter password as OpenTofu `random_password` resources; output them into `secrets.json` (mirroring `dbPassword`/`adminUiPassword`)
- [x] 2.5 Inject the AI Lab enablement, port, and secrets into the preset's `cloudinit.tf` and register the shared post-install hook in `postInstall.scripts`
- [x] 2.6 Document the three opt-in bits so Azure/Exoscale/STACKIT can adopt AI Lab by replicating them (`doc/presets.md`)

## 3. CLI surface

- [x] 3.1 Add `--with-ai-lab` flag to `exasol install` (auto-generated from the `with_ai_lab` infrastructure variable; `--ai-lab-port` likewise)
- [~] 3.2 `exasol ai-lab install` command for existing deployments — **deferred to a follow-up change** (needs a reconfigure + remote-exec flow; see design.md)
- [~] 3.3 Wire both paths to the same install logic — **deferred** with 3.2; the install-time path is complete
- [x] 3.4 Presets that do not provide `ai-lab` do not expose the `--with-ai-lab` flag, so AI Lab cannot be requested for them

## 4. Secrets and deployment metadata

- [x] 4.1 Add `aiLabScsPassword` and `aiLabJupyterPassword` (omitempty) to the `config.Secrets` struct and ensure they are read from `secrets.json`
- [x] 4.2 Add an optional AI Lab connection object to `deployment.json` and write it from the active backend only when AI Lab is installed (Go side: `DeploymentAILab` struct + `DeploymentConnection.AILab` + clone/resolve into `ConnectionInfo`; write-side: AWS `outputs.tf` emits the `aiLab` connection object and the secrets when `with_ai_lab`)
- [x] 4.3 Ensure the local backend (and any non-supporting infrastructure) omits AI Lab metadata (optional `aiLab` field; only populated when the infra writes it)

## 5. Connection information output

- [x] 5.1 Extend connection instructions / `exasol info` to show the AI Lab URL when AI Lab metadata is present, and point the user to `secrets.json` for the Jupyter and SCS master passwords (do not print the secrets), mirroring how the DB/Admin UI passwords are handled. Migrated from string-building to Go template (`connection_instructions.tmpl`) during rebase onto main (2026-06-24); `AILab`/`AILabSecured` fields added to `ConnectionDetails` in `info.go`.
- [x] 5.2 Ensure the AI Lab section is omitted when no AI Lab metadata is present, preserving SQL connection instructions

## 6. Tests

- [x] 6.1 Unit tests for preset capability resolution (`ai-lab` provided by aws / not by local)
- [x] 6.2 `--with-ai-lab` flag auto-generation verified (surfaces on `install aws`); `exasol ai-lab install` command wiring deferred with 3.2
- [x] 6.3 Unit tests for AI Lab deployment-metadata read/write (resolve present / blank-URL omitted)
- [x] 6.4 Unit tests for connection-instructions output with and without AI Lab metadata (URL shown, passwords referenced not printed)

## 7. Documentation

- [x] 7.1 Document AI Lab in the README: how to install it (`--with-ai-lab`), how to reach it, and its zero-config DB/BucketFS connection (the `exasol ai-lab install` path will be added with that command)
- [x] 7.2 Document the new exposed port, the secrets stored in `secrets.json`, and the recommendation to restrict `allowed_cidr` / use a tunnel for hardened setups

## 8. Verification

- [x] 8.1 Manual verification on a live AWS deployment — VERIFIED: `--with-ai-lab` flowed to `with_ai_lab=true`; `deployment.json` carries the `aiLab` URL; `secrets.json` holds `aiLabJupyterPassword`/`aiLabScsPassword`; SG opens 49494 and Jupyter is reachable externally; notebooks connect to the DB via the pre-seeded SCS with no manual config. Two additional bugs found and fixed post-hackathon (2026-06-24): (1) `installAiLab.sh` wrote the Quadlet file but never called `daemon-reload`/`enable`, so systemd never activated the unit; (2) Ubuntu 22.04 ships Podman 3.4.4 which predates Quadlet (4.4+) — replaced with `podman generate systemd`. Systemd unit confirmed `active (running)` and enabled on the live deployment; Jupyter reachable at `http://ec2-63-180-230-185.eu-central-1.compute.amazonaws.com:49494`.
- [x] 8.2 Manual verification that a deployment without `--with-ai-lab` shows no AI Lab section in `exasol info` and opens no AI Lab port
- [x] 8.3 Verified the AI Lab SLC tooling (`script_languages_container/export_as_is.ipynb` → `slc.export()`) works under rootless Podman. Three Podman-vs-Docker compatibility fixes were required and are now baked into `installAiLab.sh` (2026-06-24): (1) exaslct uses the Docker SDK, which needs a daemon socket at `/var/run/docker.sock` — enable the Podman `podman.socket` user unit and bind-mount it into the container; (2) Podman rejects short image names (`exasol/script-language-container:...`) with "no unqualified-search registries defined" — write a user-level `~/.config/containers/registries.conf` with `unqualified-search-registries = ["docker.io"]`; (3) `DockerRegistryImageChecker.handle_log_line` in `exasol_integration_test_docker_environment` raises on Podman's `{"status":"Already exists"}` pull responses — patch it to return `None` (treat unknown statuses as non-errors). With these, the export runs and builds images correctly.
- [ ] 8.4 Known non-blocker (NOT an Exasol Personal / AI Lab integration issue): a from-scratch SLC build of the `template-Exasol-all-python-3.10` flavor (script-languages-release 11.1.1) fails because the flavor pins exact apt versions that Ubuntu has since removed from the jammy archive (`libssl-dev=3.0.2-0ubuntu1.21`, `curl=7.81.0-1ubuntu1.23`, `openjdk-11-jdk-headless=11.0.30+7-1ubuntu1~22.04`). This is environment-independent (reproduces on stock Docker too) and is upstream flavor staleness. Reported upstream: https://github.com/exasol/script-languages-release/issues/1489. We deliberately do not work around it in `installAiLab.sh`.
