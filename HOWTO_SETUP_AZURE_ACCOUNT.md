# Set up an Azure account for Exasol Personal

This document explains how to set up an Azure account to deploy Exasol Personal.

## ✅ Prerequisites

The following procedure assumes that you have a basic understanding of how Azure works and how to manage access using Microsoft Entra ID and Azure role-based access control (RBAC). For more information, refer to the official Azure documentation: https://learn.microsoft.com/azure/

The Azure subscription must have the permissions and quota to create virtual machines, networking, managed disks, and storage accounts in the region that you want to use.

## 🛠 Procedure

### 🆕 Create an Azure account

If you do not have an Azure account, visit the Azure home page: https://azure.microsoft.com/ to create one.

In the Azure portal, open `Subscriptions`, then create a new subscription or choose an existing one for Exasol Personal deployments.

Make sure that the subscription has enough quota for the VM family and region that you plan to use.

### 🔐 In the Azure portal, do the following:

1. Choose the Azure user account that will run Exasol Personal from your local machine.
2. Open the target subscription in `Subscriptions`.
3. Open `Access control (IAM)` for that subscription.
4. Make sure that this user has permission on the target subscription.
5. Assign one of the following roles to that user at subscription scope:
   - Azure built-in `Contributor`
   - a custom role based on [assets/infrastructure/azure/rbac-role.broad.json](./assets/infrastructure/azure/rbac-role.broad.json)
   - a custom role based on [assets/infrastructure/azure/rbac-role.minimal.json](./assets/infrastructure/azure/rbac-role.minimal.json)

The easiest option is usually `Contributor` on the target subscription.
When adding a role assignment, the built-in `Contributor` role may appear under the `Privileged administrator roles` tab in the role picker.

Use only one of these options. You do not need all of them.

The custom roles are mainly for organizations that do not want to grant the built-in `Contributor` role. In that case, an Azure administrator can create one of the custom roles from this repository and assign it to the user instead.

If you want to use one of the custom role examples from this repository, replace `<subscription-id>` in the JSON file before creating the role definition in Azure.

The current Azure preset needs permission to create and delete resource groups, networking, virtual machines, managed disks, and storage accounts, and it must also be able to read storage account keys. For more information, see [Azure Infrastructure as Code Architecture](./assets/infrastructure/azure/README.md).

If your organization uses conditional access, multi-factor authentication, or approval flows in Azure, additional steps may be required. For more information, refer to the Azure documentation: https://learn.microsoft.com/azure/

### 💻 On your local machine, do the following:

1. Install the Azure CLI if it is not already installed. Follow the official installation guide for your platform: https://learn.microsoft.com/cli/azure/install-azure-cli

2. Sign in with the Azure CLI:

   ```bash
   az login
   ```

3. If you have access to more than one subscription, select the subscription that Exasol Personal should use:

   ```bash
   az account set --subscription "<subscription-id-or-name>"
   ```

4. Verify the active subscription:

   ```bash
   az account show
   ```

5. If this is a brand-new subscription and Azure reports provider registration errors during deployment, register the required resource providers:

   ```bash
   az provider register --namespace Microsoft.Compute
   az provider register --namespace Microsoft.Network
   az provider register --namespace Microsoft.Storage
   az provider register --namespace Microsoft.Resources
   ```

6. Run the launcher with the Azure preset and explicitly choose a region. For example:

   ```bash
   exasol install azure --location westeurope
   ```

The Azure preset requires `--location`, because the target region is not inferred automatically.

For more information on Azure CLI authentication and subscription selection, see:

- https://learn.microsoft.com/cli/azure/authenticate-azure-cli
- https://learn.microsoft.com/cli/azure/manage-azure-subscriptions-azure-cli
