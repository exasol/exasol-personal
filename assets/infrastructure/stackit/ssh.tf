resource "tls_private_key" "ssh_key" {
  algorithm = "RSA"
  rsa_bits  = 4096
}

resource "stackit_key_pair" "machine_key" {
  name       = "${local.deployment_id}-key"
  public_key = tls_private_key.ssh_key.public_key_openssh
}

locals {
  ssh_key_name = stackit_key_pair.machine_key.name
}
