# Set up an AWS account for Exasol Personal

This document explains how to set up an AWS account to deploy Exasol Personal.

## ✅ Prerequisites

The following procedure assumes that you have a basic understanding of how AWS works and how to set up access using AWS Identity and Access Management (IAM). For more information, refer to the official AWS documentation: https://docs.aws.amazon.com/.

The AWS account must have the permissions and quota to launch large type instances.

## 🛠 Procedure

### 🆕 Create an AWS account

If you do not have an AWS account, visit the AWS home page: https://aws.amazon.com/ to create a new account.

The AWS account must have the permissions and quota to launch large type instances.

### 🔐 In the AWS IAM console, do the following:

1. Create a new user for the Exasol instance.
2. Attach the following IAM policies to the Exasol user:
   - `AmazonEC2FullAccess`
   - `IAMReadOnlyAccess`
   - `IAMUserChangePassword`
   - `AmazonSSMFullAccess`
3. Generate AWS access keys for the user.

If you want to use multi factor authentication (MFA) or other methods for authentication in your AWS account, additional steps may be required. For more information, refer to the AWS documentation: https://docs.aws.amazon.com/.

### 💻 On your local machine, do the following:

1. Install the AWS CLI if it is not already installed. Follow the official AWS installation guide for your platform: https://docs.aws.amazon.com/cli/latest/userguide/getting-started-install.html

2. Configure a named profile `exasol` using the AWS CLI:

   ```bash
   aws configure --profile exasol
   ```

   You will be prompted for the Access Key ID abd Secret Access Key default output format:

   ```bash
   ~/dev/exasol-personal> aws configure --profile exasol
   AWS Access Key ID [None]: <your key ID>
   AWS Secret Access Key [None]: <your access key>
   Default region name [eu-west-1]: <your region>
   Default output format [None]: json
   ```

   Copy and paste the Access Key ID and Secret Access Key from the AWS IAM Console. Pick a region that such as `eu-west-1`. Choose the default output format (e.g. `json`) or leave empty.

For more information on configuring named profiles, see: https://docs.aws.amazon.com/cli/latest/userguide/cli-configure-profiles.html
