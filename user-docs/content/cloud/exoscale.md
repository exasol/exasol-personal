# Set up Exoscale

Prepare an Exoscale organization and API credentials before deploying Exasol Personal.

## Prerequisites

You need an active Exoscale organization with billing configured and sufficient quota for compute
instances, block storage volumes, and VPCs in the target zone. This procedure assumes basic
familiarity with Exoscale IAM. See the
[Exoscale documentation](https://community.exoscale.com/documentation/) for background information.

## Configure IAM access

The launcher needs an API key whose role allows the `compute`, `networking`, `sos`, and `iam`
services. The `networking` service manages the VPC that connects the nodes of a deployment.
A key of the built-in Owner role already has these permissions; to use a restricted role:

1. Create the role from the
   [minimal example policy](https://github.com/exasol/exasol-personal/blob/main/assets/infrastructure/exoscale/iam-policy.minimal.json)
   or the
   [broad example policy](https://github.com/exasol/exasol-personal/blob/main/assets/infrastructure/exoscale/iam-policy.broad.json).
   The minimal policy is recommended. The portal's role editor does not offer the `networking`
   service, so create the role, or add `networking` to an existing one, with the
   [Exoscale CLI](https://community.exoscale.com/reference/cli/exo/iam/role/) or API, which accept
   the policy file as it is. The
   [preset documentation](https://github.com/exasol/exasol-personal/blob/main/assets/infrastructure/exoscale/README.md#iam-role-and-api-key)
   shows the commands.
2. Create an API key for the role, in the portal under **IAM** > **API Keys** or with the CLI.
3. Copy the key and secret. The secret is displayed only once.

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

The supported zones are `ch-gva-2`, `de-fra-1`, `de-muc-1`, `at-vie-1`, `at-vie-2`, and `bg-sof-1`.
See the Exoscale guides for [IAM](https://community.exoscale.com/documentation/iam/) and
[API keys](https://community.exoscale.com/documentation/iam/quick-start/) for more information.

Multi-node clusters (`--cluster-size` greater than 1) connect their nodes through an Exoscale VPC
with jumbo frames, relying on the largest packet size the VPC carries, which Exoscale does not
document. If a multi-node installation times out waiting for the database while single-node
installations succeed, see
[VPC interface configuration](https://github.com/exasol/exasol-personal/blob/main/assets/infrastructure/exoscale/README.md#vpc-interface-configuration).
