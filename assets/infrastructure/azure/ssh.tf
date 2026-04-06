resource "tls_private_key" "ssh_key" {
  algorithm = "RSA"
  rsa_bits  = 4096
}

data "azurerm_client_config" "current" {}

locals {
  ssh_key_name = "${local.deployment_id}-key"
}

resource "azurerm_key_vault" "deployment" {
  name                       = substr(replace("${local.deployment_id}-kv", "_", "-"), 0, 24)
  location                   = azurerm_resource_group.rg.location
  resource_group_name        = azurerm_resource_group.rg.name
  tenant_id                  = data.azurerm_subscription.current.tenant_id
  sku_name                   = "standard"
  enable_rbac_authorization  = false
  purge_protection_enabled   = false
  soft_delete_retention_days = 7
  tags                       = local.common_tags

  access_policy {
    tenant_id = data.azurerm_subscription.current.tenant_id
    object_id = data.azurerm_client_config.current.object_id

    secret_permissions = ["Get", "Set", "Delete", "List", "Purge"]
  }
}

resource "azurerm_key_vault_secret" "ssh_private_key" {
  name         = "ssh-private-key"
  value        = tls_private_key.ssh_key.private_key_pem
  key_vault_id = azurerm_key_vault.deployment.id
  tags         = local.common_tags
}
