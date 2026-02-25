# Architecture

This document describes the high-level architecture, design philosophy, and technical decisions behind the Exasol Personal deployment tool. For implementation details, see the [Development Guide](development.md).

## Overview

The Exasol Personal tool (`exasol`) is a command-line application that automates the deployment and management of Exasol Database on cloud infrastructure. It handles infrastructure provisioning, software installation, and provides database connectivity.

**What you'll find here:**
- Design philosophy and requirements
- Technical choices and rationale
- High-level application workflow
- Cloud infrastructure approach
- Interfaces and integration points

**What's documented elsewhere:**
- [Main README](../README.md)
- [Development Guide](development.md)
- [Preset Development (contracts and schemas)](presets.md)
- Installation preset documentation (see the README in `assets/installation/<preset>/`)
- [AWS Infrastructure README](../assets/infrastructure/aws/README.md)
- [Exasol Documentation](https://docs.exasol.com/db/latest/home.htm)

## Implementation

- **Language:** Go 1.25+ (statically compiled, single binary)
- **Build System:** [Task](https://taskfile.dev/) for development automation
- **IaC Engine:** [OpenTofu](https://opentofu.org/) embedded in the binary
- **Target Platforms:** Linux, macOS (Intel & ARM), Windows (Intel & ARM)

## Design Philosophy

### Core Principles

**Simplicity**
- Minimal configuration required from users
- Sensible defaults for all parameters
- Single command to go from zero to running database

**Transparency**
- Surface operational details (IP addresses, connection strings) to users
- State and configuration stored locally for inspection
- Clear error messages with actionable guidance

**Self-Contained**
- Single binary with no external dependencies
- Infrastructure-as-Code templates embedded in the binary
- Platform-specific OpenTofu binaries bundled

**Safety**
- Explicit commands for destructive operations
- State preserved for troubleshooting
- No silent failures or data loss

**Expandability**
- Pluggable infrastructure presets per cloud provider
- Pluggable installation presets per operating system / approach
- Extensible configuration system
- Template-based customization

### User Experience Goals

- Get from installation to running database in under 5 minutes
- No need to install Terraform, cloud CLIs, or other tools
- Works on any major development platform (Linux, Mac, Windows)
- Obvious next steps at each stage
- Easy to clean up when done

## Requirements and Constraints

### Platform Support
- **Deployment platforms:** Users can run the tool on Linux, macOS, or Windows
- **Cloud targets:** Currently AWS (others can be added)
- **Distribution:** Single statically-compiled binary per platform

### Prerequisites
- **Cloud credentials:** Must be configured via environment variables (e.g., `AWS_PROFILE`)
- **Network access:** Required for cloud API calls and software downloads
- **Disk space:** Sufficient for deployment directory (~500MB)

### Design Constraints
- No external tool dependencies at runtime
- No interactive credential prompts (automation-friendly)
- State stored locally (deployment directory)
- One deployment per directory
- Deployment directory must be preserved until destruction

## Application Workflow

### Initialization Flow

The `init` command prepares a deployment directory:

1. Create deployment directory structure
2. Extract the selected infrastructure and installation presets
3. Write infrastructure variables (for example, a `.tfvars` file) and workflow state
4. Extract a platform-specific OpenTofu binary

**Output:** A self-contained deployment directory ready for provisioning.

### Deployment Flow

The `deploy` command provisions infrastructure and installs the database:

1. **Validate inputs** - Check deployment directory (state + manifests) and cloud credentials
2. **Execute Infrastructure-as-Code** - Run OpenTofu to provision cloud resources
3. **Wait for infrastructure ready** - Poll until instances are accessible
4. **Execute and monitor installation** - Run the selected installation preset and report progress
5. **Store connection information** - Save connection details locally
6. **Display instructions** - Show user how to connect

**Output:** Running Exasol database with connection information.

**Note:** An “unattended installation” style (node-local orchestration, launcher monitors only) is recommended, but not enforced. The installation preset defines what is executed and whether the launcher merely monitors or actively orchestrates steps.

### Connection Flow

The `connect` command provides database access:

1. Read connection details from deployment directory
2. Establish SQL connection to database
3. Launch interactive SQL shell

### Destruction Flow

The `destroy` command tears down all resources:

1. Read deployment state
2. Execute OpenTofu destroy to remove cloud resources
3. Clean up state files (optional)

## Cloud Infrastructure Architecture

### Infrastructure-as-Code Approach

**Why OpenTofu?**
- Declarative infrastructure definition
- Idempotent operations (safe to re-run)
- State management for tracking resources
- Provider ecosystem for cloud APIs
- Open-source alternative to Terraform

### General Infrastructure Principles

**Compute:**
- Virtual machines (EC2 instances on AWS)
- Single-node or multi-node cluster support
- Configurable instance types
- Fixed node naming scheme for consistency

**Storage:**
- Separate volumes for OS and database data
- Configurable volume sizes and types
- Data volumes encrypted

**Networking:**
- Public IP addresses for external access
- Fixed internal IP scheme for cluster communication
- Network isolation between deployments
- Firewall rules for security

**Security:**
- SSH key-based access (generated per deployment)
- Secrets stored locally with restricted permissions
- Cloud credentials from environment (never stored)

### Preset Interfaces

The Go application interacts with provisioning and installation through two preset types:

For the detailed contract (well-known paths, host file locations) and the preset manifest/task schemas, see [Preset Development](presets.md).

**Configuration (Input):**
- Deployment directory contents (preset manifests, workflow state)
- Infrastructure variables (passed to OpenTofu)
- Installation preset inputs (preset-defined; often written to nodes during bootstrap)

**Execution:**
- OpenTofu binary invoked as subprocess
- OpenTofu state managed locally
- Cloud API calls handled by OpenTofu

**Outputs (Consumed by Go app):**
- Terraform outputs (JSON) - Resource IDs, IP addresses, DNS names
- Deployment info file (JSON) - Connection details, node information
- SSH private key - For status monitoring

**Installation:**
- Implemented by the selected installation preset
- May be unattended (node-local orchestration) or launcher-driven
- Should provide clear progress and actionable failures for end users

### Infrastructure Preset Details

For detailed information about specific infrastructure presets:
- **AWS:** See [AWS Infrastructure README](../assets/infrastructure/aws/README.md)

Each infrastructure preset documents:
- Resource specifications (compute, storage, networking)
- Security configurations
- Variable definitions
- Deployment lifecycle
- Network topology

## State and Configuration Management

### Deployment Directory

The deployment directory is the central artifact containing everything needed to manage a deployment:

**Manifests and state:**
- Preset manifests (infrastructure + installation)
- Workflow state (used to detect interrupted/failed operations)

**Infrastructure-as-Code:**
- `infrastructure/` - Infrastructure preset templates
- `installation/` - Installation preset assets used during installation on the nodes

**Outputs:**
- `deployment-exasol-<deploymentId>.json` - Deployment information (IPs, connection details)
- `secrets-exasol-<deploymentId>.json` - Credentials required by the launcher (sensitive)
- `<deploymentId>.pem` - SSH private key

**Key characteristics:**
- Self-contained (portable to another machine if needed)
- Contains sensitive data (should be excluded from version control likely)
- Store state required for updates and destruction (be careful to keep it)
- Can be inspected and modified by advanced users

### Secrets Management

**Generation:**
- SSH keys generated during infrastructure provisioning
- Database passwords generated randomly
- Secrets written with restrictive file permissions (0600)

**Storage:**
- All secrets stored in deployment directory
- No secrets stored in the tool itself
- Cloud credentials read from environment variables

**Usage:**
- SSH keys used for remote script execution
- Database passwords used for connection and passed to installer

## Error Handling and Observability

### Error Handling Strategy

- **Fail fast:** Stop immediately on errors with clear messages
- **Context preservation:** Include relevant context in error messages
- **State preservation:** Maintain state even after failures for troubleshooting
- **Actionable guidance:** Suggest next steps when possible

### Logging

- **Pretty logs (TTY):** Human-readable output when running interactively
- **JSON logs (non-TTY):** Machine-readable for automation/CI
- **Verbosity control:** Configurable log levels via flags
- **No persistent logs:** Output to stdout/stderr only (user can redirect if needed)

## Extensibility

### Adding Infrastructure Presets

The architecture supports multiple infrastructure presets through a template-based approach:

1. **Create infrastructure templates** - Add Terraform files in `assets/infrastructure/<preset>/`
2. **Define infrastructure manifest** - Add preset metadata in `infrastructure.yaml`
3. **Document infrastructure** - Create a preset README
4. **Test deployment lifecycle** - Ensure init/deploy/destroy work correctly

No changes to core application logic are required - the infrastructure preset is selected via configuration.

### Adding Installation Presets

Installation presets are selected during `init` and extracted into the deployment directory.

For the developer-facing requirements (asset layout, infrastructure↔installation contract, and manifest schemas), see [Preset Development](presets.md).

### Configuration Extension

New deployment parameters can be added without code changes:

1. Add to deployment config YAML schema
2. Pass as variables to Terraform templates
3. Document in configuration reference

### Customization Points

Users and developers can customize:
- Terraform templates (after `init`, before `deploy`)
- Installation preset assets (after `init`, before `deploy`)
- Post-deployment scripts (in deployment directory)
- Configuration values (via config file or CLI flags)

## Security Considerations

### Threat Model

**Assets:**
- Cloud credentials (read from environment)
- SSH private keys (in deployment directory)
- Database credentials (in deployment directory)
- Deployment state (in deployment directory)

**Protection mechanisms:**
- Cloud credentials never stored or logged
- Deployment directory has restricted permissions
- Secrets files have 0600 permissions
- State files kept local (user's responsibility to secure)

### Network Security

- Firewall rules limit exposed services
- SSH key-based authentication only (no passwords)
- Configurable allowed CIDR ranges for access control
- Encrypted connections (SSH, TLS) for data in transit

### Best Practices

- Don't commit deployment directories to version control
- Restrict `allowed_cidr` in production deployments
- Destroy deployments when not in use
- Secure the machine running the tool

