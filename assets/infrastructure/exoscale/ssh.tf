resource "tls_private_key" "ssh_key" {
  algorithm = "RSA"
  rsa_bits  = 4096
}

resource "exoscale_ssh_key" "instance_key" {
  name       = "${local.deployment_id}-key"
  public_key = tls_private_key.ssh_key.public_key_openssh
}

locals {
  ssh_key_name = exoscale_ssh_key.instance_key.name
}
