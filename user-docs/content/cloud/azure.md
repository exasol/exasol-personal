# Set up Microsoft Azure

Prepare an Azure subscription and authenticate the Azure CLI before deploying Exasol Personal.

## Prerequisites

You need an Azure subscription with sufficient permissions and quota for virtual machines,
networking, managed disks, and storage accounts in the target region. This procedure assumes basic
familiarity with Microsoft Entra ID and Azure role-based access control (RBAC). See the
[Azure documentation](https://learn.microsoft.com/azure/) for background information.

## Configure subscription access

In the Azure portal:

1. Open the target subscription and its **Access control (IAM)** page.
2. Give the user who will run Exasol Personal one of these roles at subscription scope:
   - the built-in **Contributor** role;
   - a custom role based on the
     [broad example](https://github.com/exasol/exasol-personal/blob/v2.2.0/assets/infrastructure/azure/rbac-role.broad.json); or
   - a custom role based on the
     [minimal example](https://github.com/exasol/exasol-personal/blob/v2.2.0/assets/infrastructure/azure/rbac-role.minimal.json).

Use only one option. The built-in Contributor role is usually the simplest. When creating a custom
role from an example, replace `<subscription-id>` in the JSON first.

The role must allow the launcher to create and delete resource groups, networks, virtual machines,
managed disks, and storage accounts, and to read storage account keys. Conditional access,
multi-factor authentication, or organizational approval flows may require additional steps.

## Configure your computer

1. Install the [Azure CLI](https://learn.microsoft.com/cli/azure/install-azure-cli).
2. Sign in:

   ```bash
   az login
   ```

3. If you can access multiple subscriptions, select the one to use:

   ```bash
   az account set --subscription "<subscription-id-or-name>"
   ```

4. Verify the selection:

   ```bash
   az account show
   ```

5. For a new subscription, register these resource providers if Azure reports registration errors:

   ```bash
   az provider register --namespace Microsoft.Compute
   az provider register --namespace Microsoft.Network
   az provider register --namespace Microsoft.Storage
   az provider register --namespace Microsoft.Resources
   ```

Azure requires an explicit deployment location:

```bash
exasol install azure --location westeurope
```

For more details, see the Azure CLI guides for
[authentication](https://learn.microsoft.com/cli/azure/authenticate-azure-cli) and
[subscription selection](https://learn.microsoft.com/cli/azure/manage-azure-subscriptions-azure-cli).
