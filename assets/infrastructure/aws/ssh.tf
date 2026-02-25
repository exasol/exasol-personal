resource "tls_private_key" "ssh_key" {
  algorithm = "RSA"
  rsa_bits  = 4096
}

resource "aws_key_pair" "instance_key" {
  key_name   = "${local.deployment_id}-key"
  public_key = tls_private_key.ssh_key.public_key_openssh

  tags = {
    Name         = "${local.deployment_id}-key"
    Owner        = data.aws_caller_identity.current.arn    
  }
}

resource "aws_ssm_parameter" "ssh_private_key" {
  name  = "/${local.deployment_id}/ssh_private_key"
  type  = "SecureString"
  value = tls_private_key.ssh_key.private_key_pem

  tags = {
    Name         = "${local.deployment_id}-ssh-key"
    Owner        = data.aws_caller_identity.current.arn    
  }
}

locals {
  ssh_key_name = aws_key_pair.instance_key.key_name
}
