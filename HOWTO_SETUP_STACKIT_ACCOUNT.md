# Set up a STACKIT account for Exasol Personal

This document explains how to set up a STACKIT account to deploy Exasol Personal.

## ✅ Prerequisites

The following procedure assumes that you have a basic understanding of how STACKIT works and how to create and manage a STACKIT organization. For more information, refer to the official STACKIT documentation: https://docs.stackit.cloud/

The STACKIT account must have the permissions to create projects.

## 🛠 Procedure

### 🆕 Create a STACKIT account

If you do not have a STACKIT account, visit the STACKIT Portal: https://portal.stackit.cloud/ to create one.

In the STACKIT portal, make sure you have an active organization with billing configured.

### 🔐 In the STACKIT Portal, do the following:

1. Go to the "Resource manager".
2. Create a new project and enter it.
3. Go to the "Service Accounts" menu in the "IAM" section.
4. Create a service account and select it.
5. Copy the service account's email.
6. Create a service account key and download the JSON file.
7. Go to the "Access" menu in the "IAM" section.
8. Grant access to the service account using its email as the subject and give it the "Editor" role in the "Basic" section.
9. Copy the project's UUID from the URL or the "Resource manager".

### 💻 On your local machine, do the following:

1. Set the `STACKIT_SERVICE_ACCOUNT_KEY_PATH` environment variable to the path where you downloaded the service account credentials:

   ```bash
   # Linux / macOS (Bash)
   export STACKIT_SERVICE_ACCOUNT_KEY_PATH=/path/to/credentials.json
   ```

   ```powershell
   # Windows (PowerShell)
   $env:STACKIT_SERVICE_ACCOUNT_KEY_PATH=C:\path\to\credentials.json
   ```

   ```powershell
   # Windows (cmd)
   set STACKIT_SERVICE_ACCOUNT_KEY_PATH=C:\path\to\credentials.json
   ```

1. Run the launcher with the STACKIT preset with the `--project-id` argument. The default region is `eu01`. To deploy to a different region, pass `--region`:

```bash
exasol install stackit --project-id "<your-project-uuid>"
exasol install stackit --region eu02 --project-id "<your-project-uuid>"
```
