# Set up an Exoscale account for Exasol Personal

This document explains how to set up an Exoscale account to deploy Exasol Personal.

## ✅ Prerequisites

The following procedure assumes that you have a basic understanding of how Exoscale works and how to manage access using Exoscale IAM. For more information, refer to the official Exoscale documentation: https://community.exoscale.com/documentation/

The Exoscale organization must have the quota to create compute instances, block storage volumes, and private networks in the zone that you want to use.

## 🛠 Procedure

### 🆕 Create an Exoscale account

If you do not have an Exoscale account, visit the Exoscale home page: https://www.exoscale.com/ to create one.

In the Exoscale portal, open `Organization` and make sure you have an active organization with billing configured.

### 🔐 In the Exoscale portal, do the following:

1. Open `IAM` → `Roles` and create a new IAM role for Exasol Personal.

2. Attach a policy to the role that grants the permissions the launcher needs. This repository includes two example policies:
   - [assets/infrastructure/exoscale/iam-policy.minimal.json](./assets/infrastructure/exoscale/iam-policy.minimal.json) — least-privilege policy (recommended)
   - [assets/infrastructure/exoscale/iam-policy.broad.json](./assets/infrastructure/exoscale/iam-policy.broad.json) — broader access for dev/test environments

   Note: Exoscale IAM policies are configured via the portal, not by uploading a file. Use these JSON files as a reference for the required service and operation permissions when defining the role.

3. Open `IAM` → `API Keys` and create a new API key. Assign the IAM role you just created to this key. Copy the API key and secret — the secret is only shown once.

### 💻 On your local machine, do the following:

1. Set the Exoscale API credentials as environment variables:

   ```bash
   # Linux / macOS (Bash)
   export EXOSCALE_API_KEY=<your-api-key>
   export EXOSCALE_API_SECRET=<your-api-secret>
   ```
   ```powershell
   # Windows (PowerShell)
   $env:EXOSCALE_API_KEY = "<your-api-key>"
   $env:EXOSCALE_API_SECRET = "<your-api-secret>"
   ```
   ```powershell
   # Windows (cmd)
   set EXOSCALE_API_KEY=<your-api-key>
   set EXOSCALE_API_SECRET=<your-api-secret>
   ```

2. Run the launcher with the Exoscale preset. The default zone is `ch-gva-2`. To deploy to a different zone, pass `--zone`:

   ```bash
   exasol install exoscale                     # deploy to ch-gva-2 (default)
   exasol install exoscale --zone de-fra-1     # deploy to Frankfurt
   exasol install exoscale --zone at-vie-1     # deploy to Vienna
   ```

   Available zones: `ch-gva-2`, `de-fra-1`, `de-muc-1`, `at-vie-1`, `at-vie-2`, `bg-sof-1`.

For more information on Exoscale IAM and API keys, see:

- https://community.exoscale.com/documentation/iam/
- https://community.exoscale.com/documentation/iam/quick-start/
