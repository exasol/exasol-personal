# Glossary

This document defines key terms and concepts for consistent terminology usage across documentation, code, and discussions about Exasol Personal.

For architectural context and detailed explanations, see [Architecture](architecture.md).

## Core Concepts

### Deployment Directory
A self-contained directory with all configuration, state, and credentials for managing a specific Exasol deployment. Contains infrastructure-as-code files, state, and secrets. Must be preserved until destruction. Should not be version controlled.

See [Architecture: State and Configuration Management](architecture.md#state-and-configuration-management).

### Infrastructure Preset
The infrastructure template selected for deployment. It defines the infrastructure layout and provisioning approach. Infrastructure presets can target cloud or non-cloud environments.

### Installation Preset
The installation template that defines how software is installed on provisioned infrastructure. It should ideally be independent of the infrastructure preset and work with any.

### Active Deployment
A successfully provisioned and running Exasol deployment, including cloud infrastructure, database process, and valid state.

### Running Database
An initialized and operational Exasol database ready to accept SQL connections and queries.

## Infrastructure Components

### Node
A single compute instance (virtual machine) running Exasol database software. Has a public IP address, SSH access, and may be part of a multi-node cluster.

### Database Instance
The complete Exasol database system, including one or more nodes, storage volumes, network configuration, and security rules.

See [Architecture: Cloud Infrastructure Architecture](architecture.md#cloud-infrastructure-architecture).

## State Management

### State Files
Files tracking deployment state, including infrastructure state, SSH keys, credentials, and resource identifiers.

### Post-Deployment Scripts
Scripts executed after infrastructure provisioning to initialize the database and perform setup tasks.

## Connection Types

### Shell Connection
Secure SSH connection to a node for system-level access and infrastructure management using deployment-specific SSH keys.

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
