resource "tls_private_key" "ssh_key" {
  algorithm = "RSA"
  rsa_bits  = 4096
}

data "azurerm_client_config" "current" {}

locals {
  ssh_key_name = "${local.deployment_id}-key"
}
