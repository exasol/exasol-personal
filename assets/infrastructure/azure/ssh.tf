resource "tls_private_key" "ssh_key" {
  algorithm = "RSA"
  rsa_bits  = 4096
}

locals {
  ssh_key_name = "${local.deployment_id}-key"
}
