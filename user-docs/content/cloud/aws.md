# Set up Amazon Web Services

Prepare an AWS account and a named AWS CLI profile before deploying Exasol Personal.

## Prerequisites

You need an AWS account with the permissions and service quotas required to launch large compute
instances. This procedure assumes basic familiarity with AWS Identity and Access Management (IAM).
See the [AWS documentation](https://docs.aws.amazon.com/) for background information.

## Configure IAM access

In the AWS IAM console:

1. Create a user for Exasol Personal.
2. Attach the
   [Exasol Personal AWS policy](https://github.com/exasol/exasol-personal/blob/v2.2.0/assets/infrastructure/aws/iam-policy.broad.json)
   to the user.
3. Generate an access key for the user and store its ID and secret securely.

Additional configuration may be necessary if your organization requires multi-factor authentication
or another authentication method.

## Configure your computer

1. Install the [AWS CLI](https://docs.aws.amazon.com/cli/latest/userguide/getting-started-install.html).
2. Configure a named profile called `exasol`:

   ```bash
   aws configure --profile exasol
   ```

3. Enter the access key ID, secret access key, region, and optional output format when prompted:

   ```text
   AWS Access Key ID [None]: <your-key-id>
   AWS Secret Access Key [None]: <your-secret-key>
   Default region name [None]: eu-west-1
   Default output format [None]: json
   ```

For details, see [Named profiles for the AWS CLI](https://docs.aws.amazon.com/cli/latest/userguide/cli-configure-profiles.html).

You can now [deploy Exasol Personal to AWS](../cloud-deployment.md).
