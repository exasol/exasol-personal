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

In addition, during `init`/`install` the launcher resolves **installation preset variables** (from `installation.yaml`) and writes them to the path configured by `installation.yaml:variables:outputFile` (commonly `files/etc/exasol_launcher/installation.json`). Infrastructure presets that support unattended installation must ensure this file is copied onto the nodes (typically via the installation preset’s `files/**` overlay).

## Mandatory root-level deployment artifacts for Tofu deployments (written by infrastructure presets)

In addition to the extracted preset directories, the launcher expects the **infrastructure preset** to write a small set of artifacts into the directory specified by the Terraform variable `infrastructure_artifact_dir` - which is typically the root of the deployment directory. These artifacts are consumed by commands like `exasol info`, `exasol connect`, and `exasol diag shell`.

The launcher discovers these files by **static filename**, so both the **location (deployment root typically)** and the **filenames** are part of the contract.

Required artifacts:

- `deployment.json`
  - Purpose: non-sensitive *node details* used by the launcher (IPs/DNS, SSH connection info, DB/Admin UI endpoints, TLS cert).
  - Minimal required content (high-level): a `deploymentId` plus a `nodes` map keyed by node name (e.g. `n11`) containing at least a reachable host (`dnsName` and/or `publicIp`), SSH details (user, port, key file), DB connection details (db port, UI port, URL), and the TLS certificate PEM used for fingerprinting.

- `secrets.json`
  - Purpose: sensitive credentials used by the launcher.
  - Minimal required content: `dbPassword` and `adminUiPassword`.
  - Presets may include additional fields (for example usernames), but the launcher must be able to parse the required ones.

- `node_access.pem`
  - Purpose: SSH private key used for remote exec and diagnostic shell.
  - Requirement: must be readable by the current user and typically needs mode `0600`. `deployment.json` should reference this key via `nodes[*].ssh.keyFile` (absolute or relative paths are supported; relative paths are resolved against the deployment directory).

Reference implementation:

- The AWS infrastructure preset writes these artifacts from its OpenTofu outputs (see `assets/infrastructure/aws/outputs.tf`).

## The infrastructure <-> installation contract (well-known paths)

There is a contract between the infrastructure preset and the installation preset. The AWS infrastructure preset makes this contract explicit in `assets/infrastructure/aws/cloudinit.tf`.

The contract has three parts:

1. **How the infrastructure preset discovers installation assets** to embed into cloud-init.
2. **Where the infrastructure preset writes machine-readable configuration** on the target host.
3. **Where installation assets are placed on the target host** (paths + permissions).

In addition, for Tofu-based infrastructure presets there is a small but important contract about **launcher-injected variables** and about **what information must be surfaced onto the host** for installation scripts (via `infrastructure.json` and installation-owned configuration).

### Launcher-injected variables (internal vs user-facing)

The launcher supports two different “variable channels”, each with a clear owner:

- **Infrastructure variables (OpenTofu):** owned by the infrastructure preset and used to provision resources.
- **Installation variables (installation manifest):** owned by the installation preset and used to configure installation-time behavior on the nodes.

For OpenTofu-based infrastructure presets, the launcher generates a `.tfvars` file during `init` by combining:

- defaults declared by the infrastructure preset, and
- user overrides provided as CLI flags.

Installation presets can also declare their own variables (including defaults and help text) directly in `installation.yaml`. The launcher exposes these as CLI flags during `init`/`install`, persists the chosen values in the deployment directory, and materializes them for node-local scripts via an installation-owned config file (see below).

To keep templates composable, variables fall into two buckets:

- **Internal variables**: set by the launcher and required for correct integration. They should not be presented as user-tunable product settings.
- **User-facing variables**: exposed as CLI flags. Defaults should match the product’s intended “out of the box” behavior.

The AWS infrastructure preset documents its internal variables in `variables_internal.tf`. Infrastructure presets that want to be launcher-compatible should follow the same pattern.

#### Required internal variables (launcher provides values)

Some values are core launcher concepts (for example the deployment id and cluster identity). These values are **governed by the launcher**, persisted in the deployment directory’s launcher state, and then provided to presets via their respective variable mechanisms.

Infrastructure presets and installation presets can both depend on these values being available, but they are still expected to behave sensibly if they choose not to use them.

The launcher provides the following internal values:

- `deployment_id` (string): stable deployment identifier generated by the launcher.
- `cluster_identity` (string): stable identity string generated by the launcher (used for version-check identity and telemetry-like use cases). Presets should treat it as opaque.

In addition, for OpenTofu-based infrastructure presets the launcher provides:

- `infrastructure_artifact_dir` (string): where the preset should write the deployment artifacts consumed by the launcher.
- `installation_preset_dir` (string): where the preset can find the extracted installation preset on disk (e.g. for building cloud-init overlays).
- `deployment_created_at` (string, RFC3339): stable deployment creation timestamp generated by the launcher (useful for provider-level default tags).

Note on OpenTofu variable handling: the launcher may write additional internal variables into the generated `.tfvars` file. If an infrastructure preset does not declare a given variable, OpenTofu will typically ignore it (often with a warning). Presets that want to make use of launcher-governed values should declare them as internal variables.

Optionally, an infrastructure preset may also provide its own **host file overlay** (for example, cloud-provider-specific helper scripts) and may declare **infrastructure-specific post-install actions** in `infrastructure.json`.

In addition, there is currently a **well-known node identity convention** that crosses the preset boundary:

- The node naming scheme includes a fixed primary/access node named **`n11`**.
- Infrastructure presets are expected to provide primary-node addressing (for example `n11Ip` in `infrastructure.json`).
- Some installation presets (including `ubuntu`) use `n11` as a hard-coded leader for cluster-wide coordination and for “run only on primary” tasks.

Implications:

- Today, an infrastructure preset and an installation preset are only mix-and-match compatible if they both follow this convention.
- If you want to support a different leader election / naming scheme, treat that as a deliberate **contract change** and document it as a preset family (see “Compatibility strategies for future presets”).

### 1) How installation preset assets are discovered (Terraform-side)

The AWS infrastructure preset expects that, at apply time, the installation preset is present on disk at the value that `installation_preset_dir` defines.

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

#### Installation-owned configuration on the host

Installation presets may require additional configuration values to be available to node-local scripts.

Therefore, installation presets declare **installation variables** in `installation.yaml` and define an output path for the resolved values.

The launcher writes the resolved values into the installation preset directory in the deployment directory (at the manifest-defined path). Infrastructure presets that support unattended installation must ensure that this path is deployed to the node filesystem (for example via a host file overlay mechanism).

Installation scripts may read installation-owned configuration from that file when they are executed. In addition to explicitly declared installation variables, the launcher can include implicit core values (such as `deployment_id` and `cluster_identity`) so scripts do not need to reconstruct them.

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
- `variables` (object, optional): installation preset variables and where to write resolved values
  - `outputFile` (string, required when `variables` is present): path **relative to** `<deploymentDir>/installation/` where the launcher writes resolved values.
    - To ensure the file reaches the host, `outputFile` should point into `files/…` (for example `files/etc/exasol_launcher/installation.json`).
  - `vars` (object, required when `variables` is present): map of variable name to a variable definition.
    - Each variable definition provides at least a `description` and a `default` value.
    - `default` is required and must be a primitive scalar (`string`, `bool`, or a number).
    - `type` is optional and limited to `string`, `bool`, `number`. If omitted, the launcher infers it from `default`.
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
