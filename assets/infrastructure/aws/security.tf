# Security group for the Exasol instance
resource "aws_security_group" "exasol_instance" {
  name        = "${local.deployment_id}-sg"
  description = "Security group for Exasol Personal instance"
  vpc_id      = aws_vpc.vpc.id

  # Allow all internal VPC traffic
  ingress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = [var.vpc_cidr]
    description = "All internal VPC traffic"
  }

  # External access rules
  dynamic "ingress" {
    for_each = {
      22    = "SSH access"
      2581  = "Default bucketfs"
      8443  = "Exasol Admin UI"
      8563  = "Default Exasol database connection"
      20002 = "Exasol container ssh"
      20003 = "Exasol confd API"
    }

    content {
      from_port   = ingress.key
      to_port     = ingress.key
      protocol    = "tcp"
      cidr_blocks = [var.allowed_cidr]
      description = ingress.value
    }
  }

  # Allow all outbound traffic
  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
    description = "Allow all outbound traffic"
  }

  tags = {
    Name         = "${local.deployment_id}-sg"
    Owner        = data.aws_caller_identity.current.arn    
  }
}
