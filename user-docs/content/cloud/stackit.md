# Set up STACKIT

Prepare a STACKIT project and service account before deploying Exasol Personal.

## Prerequisites

You need an active STACKIT organization with billing configured and permission to create projects.
See the [STACKIT documentation](https://docs.stackit.cloud/) for background information.

## Configure a service account

In the STACKIT Portal:

1. Open **Resource manager**, create a project, and enter it.
2. Open **IAM** > **Service Accounts** and create a service account.
3. Copy the service account email.
4. Create a service account key and download the JSON credentials file.
5. Open **IAM** > **Access**.
6. Add the service account email as the subject and grant the **Editor** role from the **Basic**
   section.
7. Copy the project UUID from the Resource manager or the project URL.

## Configure your computer

Set `STACKIT_SERVICE_ACCOUNT_KEY_PATH` to the downloaded credentials file.

On Linux or macOS:

```bash
export STACKIT_SERVICE_ACCOUNT_KEY_PATH=/path/to/credentials.json
```

In Windows PowerShell:

```powershell
$env:STACKIT_SERVICE_ACCOUNT_KEY_PATH = "C:\path\to\credentials.json"
```

In Windows Command Prompt:

```batch
set STACKIT_SERVICE_ACCOUNT_KEY_PATH=C:\path\to\credentials.json
```

Install with the project UUID. The default region is `eu01`; use `--region` to select another:

```bash
exasol install stackit --project-id "<your-project-uuid>"
exasol install stackit --region eu02 --project-id "<your-project-uuid>"
```
