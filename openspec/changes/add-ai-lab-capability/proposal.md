## Why

Exasol Personal users who want to do data science / AI work against their database currently have to find, install, and manually wire up a notebook environment themselves — installing a container runtime, running the Exasol AI Lab image, and hand-entering database and BucketFS connection details. This is exactly the friction Exasol Personal exists to remove. Because a cloud deployment already provisions a capable compute instance running the database, we can co-locate the AI Lab there and pre-wire it, so users get a ready-to-use Jupyter environment connected to their data with zero configuration.

## What Changes

- Add an optional **Exasol AI Lab** installation that runs the official `exasol/ai-lab` container (via Podman) on the same infrastructure (e.g. the AWS EC2 instance) that hosts the Exasol database.
- Make AI Lab installation **opt-in** via a `--with-ai-lab` flag on `exasol install`. (A standalone `exasol ai-lab install` command to add AI Lab to an already-running deployment is **deferred to a follow-up change** — it requires a reconfigure + remote-exec flow that is out of scope here; see design.md.)
- **Pre-configure** AI Lab so it connects to the local Exasol database and BucketFS automatically, with no manual configuration steps: the launcher seeds AI Lab's Secure Configuration Storage (SCS) with the resolved DB and BucketFS connection parameters.
- **Auto-generate** the SCS master password and Jupyter password as OpenTofu `random_password` outputs and store them in `secrets.json`, alongside the existing `dbPassword`/`adminUiPassword` (mirroring how those secrets are handled today).
- **Expose** the AI Lab (Jupyter) port through a dedicated security-group ingress rule, gated by the existing `allowed_cidr` restriction and protected by the Jupyter password.
- **Print connection information** for AI Lab in `exasol info` / connection instructions — the AI Lab URL, with the Jupyter and SCS master passwords referenced via `secrets.json` rather than printed — shown only when AI Lab is present for that deployment.
- Track the `exasol/ai-lab:latest` container image.
- Restrict availability to infrastructure presets that declare support: AI Lab is offered on cloud presets only. **Local deployments do not get AI Lab** in this version because UDFs and BucketFS are not available locally; the capability is designed so local can be added later without spec changes to consumers.

## Capabilities

### New Capabilities
- `ai-lab-access`: how the launcher optionally installs the Exasol AI Lab container alongside the database on supporting infrastructure, pre-configures its connection to the database and BucketFS, exposes and secures its port, and surfaces AI Lab connection information — while omitting it on infrastructure that does not support it (e.g. local).

### Modified Capabilities
<!-- None: no existing permanent spec covers AI Lab. The preset-declares-support, deployment.json connection metadata, and conditional connection-instructions behaviors are captured within the new ai-lab-access capability, consistent with how admin-ui-access is structured. -->

## Impact

The work divides along the existing preset boundary: the cloud-agnostic mechanics live once in the **installation (OS) preset**, and each **infrastructure preset** opts in with a small, provider-specific surface. AWS is the first infrastructure preset to opt in; Azure/Exoscale/STACKIT can follow later by adding the same three bits.

- **Installation (OS) preset — shared across all clouds** (`assets/installation/ubuntu/`): install Podman; add a post-install hook **script** (alongside the existing shared scripts like `installExasol.sh` / `runInfraHookScripts.sh`) that pulls and runs the `exasol/ai-lab` container, sets the Jupyter password, generates the SCS master password, seeds the SCS with DB + BucketFS connection parameters, and registers a systemd/Podman unit (with user lingering) so AI Lab survives reboots and start/stop. This runs identically regardless of cloud.
- **Infrastructure presets — per cloud, AWS first** (`assets/infrastructure/aws/`): the only inherently provider-specific pieces — (1) declare `ai-lab` in `compatibility.provides`; (2) add a security-group ingress rule for the AI Lab port gated by `allowed_cidr` (provider-specific: AWS SG ≠ Azure NSG ≠ Exoscale/STACKIT); (3) inject the AI Lab settings/secrets into the preset's `cloudinit.tf` and register the shared hook in `postInstall.scripts`. Other cloud presets opt in by adding these same three bits.
- **CLI** (`cmd/exasol/`): add the `--with-ai-lab` flag to `install`; add an `exasol ai-lab install` command; extend connection-instructions/`info` output to include AI Lab access.
- **Deployment metadata**: add an optional AI Lab connection object to `deployment.json`; generate the SCS master password and Jupyter password as OpenTofu `random_password` outputs and store them in `secrets.json` (mirroring `dbPassword`/`adminUiPassword`); add the corresponding fields to the `config.Secrets` struct.
- **Backends** (`internal/deploy`): write AI Lab connection metadata when the infrastructure exposes it; omit it (e.g. for the local backend) otherwise.
- **Variables**: infrastructure-preset variables `with_ai_lab` (enablement) and `ai_lab_port`. Enablement is an infrastructure variable rather than an installation-preset variable because it must be known at Terraform plan time (to conditionally create the security-group rule, secrets, outputs, and hook registration); it is surfaced to the host via `infrastructure.json`. Both auto-surface as the `--with-ai-lab` / `--ai-lab-port` CLI flags.
- **Documentation**: README and connection docs describe AI Lab, how to reach it, and its zero-config DB/BucketFS connection.
- **External dependencies**: the `exasol/ai-lab` container image (pulled at deploy time) and the `podman` package on the host. No new Go dependencies.
- **Security**: a new exposed port and two generated secrets (Jupyter + SCS master passwords) stored in `secrets.json`; the port is gated by `allowed_cidr` and the secrets follow the existing storage model. Connection instructions show the AI Lab URL and point to `secrets.json` for the passwords rather than printing them.
