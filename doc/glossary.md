# Glossary

This document defines the internal terms contributors use across code, contributor documentation, and design discussions. For architectural context and detailed explanations, see [Architecture](architecture.md).

## Shared Terms

Terms that users and contributors share have a single definition in the [user glossary](../user-docs/content/glossary.md): [Exasol Personal](../user-docs/content/glossary.md#exasol-personal), [Exasol Launcher](../user-docs/content/glossary.md#exasol-launcher), [deployment](../user-docs/content/glossary.md#deployment), [deployment directory](../user-docs/content/glossary.md#deployment-directory), [node](../user-docs/content/glossary.md#node), [preset](../user-docs/content/glossary.md#preset), [infrastructure preset](../user-docs/content/glossary.md#infrastructure-preset), and [installation preset](../user-docs/content/glossary.md#installation-preset). Use those definitions in contributor material too, rather than restating them here. For how the launcher uses the deployment directory internally, see [Architecture: State and Configuration Management](architecture.md#state-and-configuration-management).

## Deployment Internals

### Deployment Version
The launcher version that was persisted when a deployment directory was created. It is an immutable marker that defines the deployment’s compatibility contract.

See [Deployment directory compatibility](deployment_compatibility.md).

### Deployment Compatibility Check
A mandatory validation performed before executing any command that operates on a deployment directory. It ensures the deployment version is compatible with the current launcher version and the command’s supported range.

See [Deployment directory compatibility](deployment_compatibility.md).

### Workflow State
The lifecycle state a deployment directory records between command invocations, and what `status` reports.

See [Deployment state & locking](launcher_state.md).

### State Files
The files through which the launcher carries deployment state across invocations: the persistent workflow state, the temporary deployment lock, and the plain-text deployment version marker.

See [Deployment state & locking](launcher_state.md).

### Post-Deployment Scripts
Scripts an installation preset executes on provisioned infrastructure to initialize the database and perform setup tasks. They may run node-local (unattended) or be driven by the launcher.

See [Preset development](presets.md#installation-preset).

### Database Instance
The complete Exasol database system of one deployment: its nodes together with their storage, network configuration, and access rules.

See [Architecture: Cloud Infrastructure Architecture](architecture.md#cloud-infrastructure-architecture).

## Connection Types

### Shell Connection
Interactive system-level access to a deployment’s node, over SSH with deployment-specific keys on cloud deployments.

### SQL Connection
Database connection using the database protocol and credentials for query execution and data management.

## Security

### Database Credentials
Authentication details for database access (username, password, SSL/TLS settings).

### Infrastructure Credentials
Authentication details for infrastructure access (SSH keys, cloud provider credentials, certificates).

See [Architecture: Security Considerations](architecture.md#security-considerations).

## Terms to Avoid

To maintain clarity, avoid these ambiguous terms in favor of the defined terms above:
- "environment" (use "deployment" instead)
- "instance" alone (specify "database instance" or "compute instance/node")
- "configuration" alone (specify "database configuration" or "infrastructure configuration")
- "connection" alone (specify "shell connection" or "SQL connection")
