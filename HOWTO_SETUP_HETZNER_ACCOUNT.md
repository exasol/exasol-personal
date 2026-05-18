# Set up a Hetzner Cloud account for Exasol Personal

This document explains how to set up a Hetzner Cloud account to deploy Exasol Personal.

## ✅ Prerequisites

The following procedure assumes that you have a basic understanding of how Hetzner Cloud works and how to manage projects and API tokens. For more information, refer to the official Hetzner Cloud documentation: https://docs.hetzner.com/cloud/

The Hetzner Cloud project must have sufficient quota to create servers, volumes, networks, and firewalls in the location that you want to use.

## 🛠 Procedure

### 🆕 Create a Hetzner Cloud account

If you do not have a Hetzner Cloud account, visit the Hetzner Cloud home page: https://www.hetzner.com/cloud/ to create one.

In the Hetzner Cloud Console, create a new project or choose an existing one for Exasol Personal deployments.

### 🔐 In the Hetzner Cloud Console, do the following:

1. Open the project that you want to use for Exasol Personal.
2. Navigate to `Security` → `API Tokens`.
3. Click `Generate API Token`.
4. Give the token a descriptive name (e.g., `exasol-personal`).
5. Select **Read & Write** permissions.
6. Click `Generate API Token` and copy the token — it is only shown once.

**Note:** SSH keys are automatically created during deployment — you do not need to pre-add any SSH keys to your Hetzner project.

### 💻 On your local machine, do the following:

1. Set the Hetzner Cloud API token as an environment variable:

   ```bash
   # Linux / macOS (Bash)
   export HCLOUD_TOKEN=<your-api-token>
   ```
   ```powershell
   # Windows (PowerShell)
   $env:HCLOUD_TOKEN = "<your-api-token>"
   ```
   ```powershell
   # Windows (cmd)
   set HCLOUD_TOKEN=<your-api-token>
   ```

2. Run the launcher with the Hetzner preset. The default location is `fsn1`. To deploy to a different location, pass `--location`:

   ```bash
   exasol install hetzner                      # deploy to fsn1 (default)
   exasol install hetzner --location nbg1      # deploy to Nuremberg
   exasol install hetzner --location hel1      # deploy to Helsinki
   exasol install hetzner --location ash       # deploy to Ashburn, US
   exasol install hetzner --location hil       # deploy to Hillsboro, US
   ```

   Available locations: `fsn1` (Falkenstein), `nbg1` (Nuremberg), `hel1` (Helsinki), `ash` (Ashburn), `hil` (Hillsboro).

For more information on Hetzner Cloud API tokens and authentication, see:

- https://docs.hetzner.com/cloud/api/getting-started/generating-api-token/
- https://docs.hetzner.com/cloud/api/getting-started/
