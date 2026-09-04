# Set up Exoscale

Prepare an Exoscale organization and API credentials before deploying Exasol Personal.

## Prerequisites

You need an active Exoscale organization with billing configured and sufficient quota for compute
instances, block storage volumes, and private networks in the target zone. This procedure assumes
basic familiarity with Exoscale IAM. See the
[Exoscale documentation](https://community.exoscale.com/documentation/) for background information.

## Configure IAM access

In the Exoscale portal:

1. Open **IAM** > **Roles** and create a role for Exasol Personal.
2. Grant the permissions described by the
   [minimal example policy](https://github.com/exasol/exasol-personal/blob/v2.2.0/assets/infrastructure/exoscale/iam-policy.minimal.json)
   or the
   [broad example policy](https://github.com/exasol/exasol-personal/blob/v2.2.0/assets/infrastructure/exoscale/iam-policy.broad.json).
   The minimal policy is recommended. Use the JSON as a reference when entering the permissions in
   the portal; Exoscale does not import the file directly.
3. Open **IAM** > **API Keys**, create an API key, and assign the new role.
4. Copy the key and secret. The secret is displayed only once.

## Configure your computer

Set the credentials as environment variables.

On Linux or macOS:

```bash
export EXOSCALE_API_KEY=<your-api-key>
export EXOSCALE_API_SECRET=<your-api-secret>
```

In Windows PowerShell:

```powershell
$env:EXOSCALE_API_KEY = "<your-api-key>"
$env:EXOSCALE_API_SECRET = "<your-api-secret>"
```

In Windows Command Prompt:

```batch
set EXOSCALE_API_KEY=<your-api-key>
set EXOSCALE_API_SECRET=<your-api-secret>
```

The default zone is `ch-gva-2`. Select another zone with `--zone`:

```bash
exasol install exoscale
exasol install exoscale --zone de-fra-1
exasol install exoscale --zone at-vie-1
```

Release 2.2 supports `ch-gva-2`, `de-fra-1`, `de-muc-1`, `at-vie-1`, `at-vie-2`, and `bg-sof-1`.
See the Exoscale guides for [IAM](https://community.exoscale.com/documentation/iam/) and
[API keys](https://community.exoscale.com/documentation/iam/quick-start/) for more information.
