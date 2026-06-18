## 1. Installation (OS) preset — shared across all clouds

- [x] 1.1 Install the `podman` package on the host during preparation (in the `ubuntu` installation preset)
- [x] 1.2 Add a post-install hook script (alongside the existing shared scripts, e.g. `installExasol.sh`/`runInfraHookScripts.sh`) that pulls the latest `exasol/ai-lab` image and runs it via Podman, publishing the AI Lab port and mounting a persistent notebooks volume
- [x] 1.3 Set the Jupyter password on the container from the value injected by the infrastructure preset
- [x] 1.4 Generate/seed the SCS: write the master password and the database connection parameters (host, port, user, password, encryption, `cert_vld=false`, `storage_backend=onprem`, `use_itde=false`) and the BucketFS parameters (`bfs_*`) using the notebook-connector configuration keys
- [ ] 1.5 Confirm the seeded SCS file name/location matches what the AI Lab notebooks load by default, and that connection works against the self-signed DB certificate (needs live verification)
- [x] 1.6 Install a Podman Quadlet/systemd unit and enable user lingering so AI Lab restarts after reboot and survives deployment stop/start
- [ ] 1.7 Verify the AI Lab container does not interfere with the C4-managed database container (rootless isolation) (needs live verification)
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

- [x] 5.1 Extend connection instructions / `exasol info` to show the AI Lab URL when AI Lab metadata is present, and point the user to `secrets.json` for the Jupyter and SCS master passwords (do not print the secrets), mirroring how the DB/Admin UI passwords are handled
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

- [ ] 8.1 Manual verification on a live AWS deployment: `--with-ai-lab` installs AI Lab; the URL is reachable within `allowed_cidr`; the Jupyter password from `secrets.json` works; notebooks connect to the DB and BucketFS with no manual configuration
- [ ] 8.2 Manual verification that a deployment without `--with-ai-lab` shows no AI Lab section in `exasol info` and opens no AI Lab port
