resource "tls_private_key" "ssh_key" {
  algorithm = "RSA"
  rsa_bits  = 4096
}

data "azurerm_client_config" "current" {}

data "azuread_user" "current" {
  object_id = data.azurerm_client_config.current.object_id
}

# resource "azurerm_key_vault" "deployment" {
#   name                       = substr(replace("${local.deployment_id}-kv", "_", "-"), 0, 24)
#   location                   = azurerm_resource_group.rg.location
#   resource_group_name        = azurerm_resource_group.rg.name
#   tenant_id                  = data.azurerm_subscription.current.tenant_id
#   sku_name                   = "standard"
#   enable_rbac_authorization  = true
#   purge_protection_enabled   = false
#   soft_delete_retention_days = 7
#   tags                       = local.common_tags
# }

# resource "azurerm_role_assignment" "kv_secrets_officer" {
#   scope                = azurerm_key_vault.deployment.id
#   role_definition_name = "Key Vault Secrets Officer"
#   principal_id         = data.azurerm_client_config.current.object_id
# }

# resource "azurerm_key_vault_secret" "ssh_private_key" {
#   name         = "ssh-private-key"
#   value        = tls_private_key.ssh_key.private_key_pem
#   key_vault_id = azurerm_key_vault.deployment.id
#
#   depends_on = [azurerm_role_assignment.kv_secrets_officer]
# }

locals {
  ssh_key_name = "${local.deployment_id}-key"
}
