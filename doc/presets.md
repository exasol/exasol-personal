# Preset development (infrastructure + installation)

This document describes what a developer needs to know to add or modify **infrastructure presets** and **installation presets**.

It complements the high-level [Architecture](architecture.md) and terminology in the [Glossary](glossary.md).

## Two preset types

### Infrastructure preset

An infrastructure preset provisions resources (nodes, disks, network) and prepares node bootstrap data.

In this repository, infrastructure presets are implemented as OpenTofu templates embedded in the binary and extracted into the deployment directory during `init`.

### Installation preset

An installation preset defines *how* Exasol is installed and configured on provisioned nodes.

An installation preset may be fully launcher-driven (remote orchestration), fully unattended (node-local orchestration + launcher monitoring), or a hybrid. The launcher treats the preset as the source of truth for what happens during installation.

## Where presets live (repository layout)

- `assets/infrastructure/<preset>/` – embedded infrastructure presets (OpenTofu templates + `infrastructure.yaml`).
- `assets/installation/<preset>/` – embedded installation presets (scripts, cloud-init fragments, system assets + `installation.yaml`).

During `init`, the selected presets are extracted into the **deployment directory**:

- `<deploymentDir>/infrastructure/` – extracted infrastructure preset.
- `<deploymentDir>/installation/` – extracted installation preset.

The deployment directory layout matters because infrastructure code often references installation assets by *relative path*.

## Mandatory root-level deployment artifacts for Tofu deployments (written by infrastructure presets)

In addition to the extracted preset directories, the launcher expects the **infrastructure preset** to write a small set of artifacts into the directory spefied by the Terraform variable `infrastructure_artifact_dir` - which is typically the root of deployment directory. These artifacts are consumed by commands like `exasol info`, `exasol connect`, and `exasol diag shell`.

The current launcher implementation discovers these files by glob pattern, so both the **location (deployment root typically)** and the **filename pattern** are part of the contract.

Required artifacts:

- `deployment-exasol-<deploymentId>.json`
  - Purpose: non-sensitive *node details* used by the launcher (IPs/DNS, SSH connection info, DB/Admin UI endpoints, TLS cert).
  - Minimal required content (high-level): a `deploymentId` plus a `nodes` map keyed by node name (e.g. `n11`) containing at least a reachable host (`dnsName` and/or `publicIp`), SSH details (user, port, key file), DB connection details (db port, UI port, URL), and the TLS certificate PEM used for fingerprinting.

- `secrets-exasol-<deploymentId>.json`
  - Purpose: sensitive credentials used by the launcher.
  - Minimal required content: `dbPassword` and `adminUiPassword`.
  - Presets may include additional fields (for example usernames), but the launcher must be able to parse the required ones.

- `<deploymentId>.pem`
  - Purpose: SSH private key used for remote exec and diagnostic shell.
  - Requirement: must be readable by the current user and typically needs mode `0600`. The `deployment-…json` should reference this key via `nodes[*].ssh.keyFile` (absolute or relative paths are supported; relative paths are resolved against the deployment directory).

Reference implementation:

- The AWS infrastructure preset writes these artifacts from its OpenTofu outputs (see `assets/infrastructure/aws/outputs.tf`).

## The infrastructure <-> installation contract (well-known paths)

There is a contract between the infrastructure preset and the installation preset. The AWS infrastructure preset makes this contract explicit in `assets/infrastructure/aws/cloudinit.tf`.

The contract has three parts:

1. **How the infrastructure preset discovers installation assets** to embed into cloud-init.
2. **Where the infrastructure preset writes machine-readable configuration** on the target host.
3. **Where installation assets are placed on the target host** (paths + permissions).

Optionally, an infrastructure preset may also provide its own **host file overlay** (for example, cloud-provider-specific helper scripts) and may declare **infrastructure-specific post-install actions** in `infrastructure.json`.

In addition, there is currently a **well-known node identity convention** that crosses the preset boundary:

- The node naming scheme includes a fixed primary/access node named **`n11`**.
- Infrastructure presets are expected to provide primary-node addressing (for example `n11Ip` in `infrastructure.json`).
- Some installation presets (including `ubuntu`) use `n11` as a hard-coded leader for cluster-wide coordination and for “run only on primary” tasks.

Implications:

- Today, an infrastructure preset and an installation preset are only mix-and-match compatible if they both follow this convention.
- If you want to support a different leader election / naming scheme, treat that as a deliberate **contract change** and document it as a preset family (see “Compatibility strategies for future presets”).

### 1) How installation preset assets are discovered (Terraform-side)

The AWS infrastructure preset expects that, at apply time, the installation preset is present on disk at:

- `${path.module}/../installation`

Implications:

- This only works if the infrastructure preset is extracted into `<deploymentDir>/infrastructure/` and the installation preset into `<deploymentDir>/installation/`.
- If you change the deployment directory structure, you must update the infrastructure preset templates accordingly.

Within that installation preset directory, AWS currently expects two subdirectories:

- `cloudconf/` – **cloud-config YAML parts** (flat directory)
- `files/` – a **filesystem overlay** copied to the instance via cloud-init `write_files`

The AWS preset loads these with globs:

- `fileset(<installation>/cloudconf, "*")` (flat, lexicographically ordered)
- `fileset(<installation>/files, "**")` (recursive)

Practical guidance:

- Keep `cloudconf/` flat and stable; if you need ordering, encode it in filenames (e.g. numeric prefixes).
- Treat `files/` as a “root filesystem tree”; paths under it become absolute paths on the host.

### 2) Well-known configuration files written on the host

The AWS preset writes two JSON files on every node (via cloud-init `write_files`):

- `/etc/exasol_launcher/infrastructure.json` – deployment-wide metadata
- `/etc/exasol_launcher/node.json` – node-specific metadata (identity, per-node values)

These files are the *primary interface* from infrastructure provisioning into installation scripts.

Optional extensions used by some presets:

- `.postInstall.scripts` (array of absolute paths): an ordered list of infrastructure-specific post-install scripts to execute on the access node. Installation presets can support this via a generic runner invoked by their post-install phase.
- `.preInstall.root.scripts` (array of absolute paths): an ordered list of infrastructure-specific scripts to run during the root preparation phase.
- `.preInstall.user.scripts` (array of absolute paths): an ordered list of infrastructure-specific scripts to run during the user preparation phase.

Implications:

- Installation scripts should treat these files as authoritative configuration.
- Infrastructure presets should keep the JSON content **backwards compatible** (or version and migrate) if they expect existing installation presets to keep working.

Security note:

- The AWS payload currently includes sensitive material (e.g., SSH private key, DB/AdminUI passwords, TLS private key). If you extend this pattern, be deliberate about permissions and avoid logging these files.

### 3) How installation preset files map onto the host filesystem

The AWS preset copies every file under `installation/files/**` onto the host, preserving relative paths as absolute paths:

- `installation/files/<relpath>` -> `/<relpath>`

Permissions are set by convention:

- `*.sh` files are written as executable (`0755`)
- everything else is written as `0644`

Implications:

- If your preset needs non-shell executables or stricter permissions, you must either:
  - adjust the infrastructure preset behavior, or
  - implement permission-fixing as part of your bootstrap/installation workflow.

Infrastructure preset overlay (optional):

- An infrastructure preset may also copy files onto the host (for example from a `files/` directory in the infrastructure preset).
- If both overlays write to the same target path, the infrastructure preset should define a deterministic precedence (for example, applying infrastructure files after installation files).

## Cloud-init composition behavior (AWS preset)

The AWS preset constructs multipart cloud-init user-data as:

1. One part per file in `installation/cloudconf/*` (stable lexicographic order)
2. A final part that appends `write_files` for JSON config and `installation/files/**`

The final part is intentionally last so it can override earlier cloud-config values.

If you build a new infrastructure preset, you can follow the same pattern to keep installation presets portable.

## Preset manifest schemas (what the launcher requires)

The launcher reads two manifest files from the deployment directory:

- `<deploymentDir>/infrastructure/infrastructure.yaml`
- `<deploymentDir>/installation/installation.yaml`

Unknown keys are tolerated for forward compatibility, but the documented keys below are the ones the launcher currently understands.

### `infrastructure.yaml`

Minimal schema:

- `name` (string, required): human-friendly name shown to users
- `description` (string, required): longer description
- `tofu` (object, optional): OpenTofu-related configuration
  - `variablesFile` (string): variables file name (default: `variables.tf` when `tofu` is present)
  - `varsOutputFile` (string): where `init` writes the generated `.tfvars` (default: `vars.tfvars`)  

Practical implications:

- If `tofu` is omitted, the launcher will skip provisioning using tofu.

### `installation.yaml`

Minimal schema:

- `name` (string, required)
- `description` (string, required)
- `install` (list or single object, required): list of installation steps

Each step currently supports the **remote exec** task type.

Remote exec step schema:

- `description` (string): message shown to the user
- `filename` (string): script path **relative to** `<deploymentDir>/installation/`
- `node` (string): node selector (e.g. `n11`)
- `executeInParallel` (bool): whether the launcher may run it in parallel with other tasks
- `regexLog` (list): optional structured log extraction
  - `regex` (string, required): compiled at manifest load time (invalid regex fails)
  - `message` (string): message template; capture groups can be referenced (e.g. `$1`)
  - `logAsError` (bool): whether matches should be surfaced as errors

YAML convenience features:

- Steps may be written in nested form (`- remoteExec: { ... }`) or flattened form (fields directly under `-`).
- `install` may be written as a YAML list or as a single object (it will be wrapped into a one-item list).

## Compatibility strategies for future presets

If you want infrastructure and installation presets to remain mix-and-match, treat the contract above as a **platform interface**.

If you intentionally break the contract (different host paths, different discovery layout, different metadata files), you can still do so by defining a **compatible subset** (a “preset family”) where infrastructure and installation presets are designed together.

When you do that, document the differences in the preset README(s) and ensure the launcher integration remains clear (especially around monitoring, progress reporting, and failure modes).
