# Generate a unique ID for this deployment
resource "random_id" "deployment_id" {
  byte_length = 4
  prefix      = "exasol-"
}

locals {
  deployment_id = random_id.deployment_id.hex

  # Node configuration
  node_start_num = 11
  nodes = [
    for i in range(var.cluster_size) : {
      name = "n${local.node_start_num + i}"
      ip   = "172.30.1.${local.node_start_num + i}"
    }
  ]

  selected_ami_id = var.ami_id != "" ? var.ami_id : data.aws_ami.ubuntu.id

  # Final passwords: prefer user-provided over generated
  db_password_final      = var.db_password != "" ? var.db_password : random_password.db.result
  adminui_password_final = var.adminui_password != "" ? var.adminui_password : random_password.adminui.result
}

# Get current region
data "aws_region" "current" {}

# Get caller identity for automatic owner tagging
data "aws_caller_identity" "current" {}

# Find latest Ubuntu AMI (DISABLED)
data "aws_ami" "ubuntu" {
  most_recent = true
  filter {
    name   = "name"
    values = ["ubuntu/images/hvm-ssd/ubuntu-${var.ubuntu_version}-*-amd64-server-*"]
  }
  filter {
    name   = "virtualization-type"
    values = ["hvm"]
  }
  owners = [var.ubuntu_owner_id]
}

# Get availability zones that support the chosen instance type
data "aws_ec2_instance_type_offerings" "available_zones" {
  filter {
    name   = "instance-type"
    values = [var.instance_type]
  }

  location_type = "availability-zone"
}

# Randomly select one AZ from those that support the instance type
# This distributes load across AZs instead of everyone using the first one
resource "random_shuffle" "az_selection" {
  input        = data.aws_ec2_instance_type_offerings.available_zones.locations
  result_count = 1

  # Preserve the selected AZ for the lifetime of this deployment to avoid AZ drift.
  # Without this, subsequent applies could re-shuffle and pick a different AZ, causing
  # EBS volumes (locked to original AZ) to become detached from newly-created instances.
  # This is critical for start/stop operations that recreate instances while preserving volumes.
  lifecycle {
    ignore_changes = [input]
  }
}

# Fetch specs of specified instance type from AWS
data "aws_ec2_instance_type" "instance" {
    instance_type = var.instance_type
}

locals {
  instance_vcpus = data.aws_ec2_instance_type.instance.default_vcpus
  instance_ram_gb = data.aws_ec2_instance_type.instance.memory_size / 1024
}

locals {
  selected_az = random_shuffle.az_selection.result[0]
}

# VPC and networking
resource "aws_vpc" "vpc" {
  cidr_block           = var.vpc_cidr
  enable_dns_hostnames = true
  enable_dns_support   = true

  tags = {
    Name         = "${local.deployment_id}-vpc"
    Owner        = data.aws_caller_identity.current.arn
  }
}

resource "aws_subnet" "subnet" {
  vpc_id                  = aws_vpc.vpc.id
  cidr_block              = var.subnet_cidr
  map_public_ip_on_launch = true

  availability_zone = local.selected_az

  tags = {
    Name         = "${local.deployment_id}-subnet"
    Owner        = data.aws_caller_identity.current.arn
  }

  lifecycle {
    precondition {
      condition     = length(data.aws_ec2_instance_type_offerings.available_zones.locations) > 0
      error_message = "Instance type ${var.instance_type} is not available in any availability zone in this region. Please choose a different instance type or region."
    }
  }
}

resource "aws_internet_gateway" "gateway" {
  vpc_id = aws_vpc.vpc.id

  tags = {
    Name         = "${local.deployment_id}-igw"
    Owner        = data.aws_caller_identity.current.arn
  }
}

resource "aws_route_table" "route_table" {
  vpc_id = aws_vpc.vpc.id

  route {
    cidr_block = "0.0.0.0/0"
    gateway_id = aws_internet_gateway.gateway.id
  }

  tags = {
    Name         = "${local.deployment_id}-rt"
    Owner        = data.aws_caller_identity.current.arn
  }
}

resource "aws_route_table_association" "route_table_assoc" {
  subnet_id      = aws_subnet.subnet.id
  route_table_id = aws_route_table.route_table.id
}

# Archive volume s3 bucket
# We use a uuid as the s3 bucket name as it must be globally unique
# If a user needs to find the bucket, they can use the tag containing the deployment id
resource "random_uuid" "archive_bucket_uuid" {}

locals {
  archive_bucket_tag_name = "${local.deployment_id}-s3-archive"
  archive_bucket_id = "${local.archive_bucket_tag_name}-${random_uuid.archive_bucket_uuid.result}"
}

resource "aws_s3_bucket" "remote_archive_volume" {
  count  = var.s3_archive_enabled ? 1 : 0
  bucket = local.archive_bucket_id

  # Without this, a bucket that isn't empty can't be deleted by Terraform
  force_destroy = true 

  tags = {
    Name         = local.archive_bucket_tag_name
    Owner        = data.aws_caller_identity.current.arn
  }
}

# Optional: create an S3 Gateway VPC endpoint if deploying into private networks
resource "aws_vpc_endpoint" "s3_gateway" {
  count               = var.s3_archive_enabled ? 1 : 0
  vpc_id              = aws_vpc.vpc.id
  service_name        = "com.amazonaws.${data.aws_region.current.id}.s3"
  vpc_endpoint_type   = "Gateway"
  route_table_ids     = [aws_route_table.route_table.id]

  tags = {
    Name  = "${local.deployment_id}-s3-endpoint"
    Owner = data.aws_caller_identity.current.arn
  }
}

## Archive IAM resources are created only when archive setup is enabled
resource "aws_iam_role" "exasol_instance_role" {
  count = var.s3_archive_enabled ? 1 : 0
  name               = "${local.deployment_id}-instance-role"
  assume_role_policy = jsonencode({
    Version = "2012-10-17",
    Statement = [
      {
        Effect = "Allow",
        Principal = {
          Service = "ec2.amazonaws.com"
        },
        Action = "sts:AssumeRole"
      }
    ]
  })
  tags = {
    Name  = "${local.deployment_id}-instance-role"
    Owner = data.aws_caller_identity.current.arn
  }
}

resource "aws_iam_role_policy" "exasol_instance_role_s3" {
  count  = var.s3_archive_enabled ? 1 : 0
  name   = "${local.deployment_id}-s3-access"
  role   = aws_iam_role.exasol_instance_role[0].id
  policy = jsonencode({
    Version = "2012-10-17",
    Statement = [
      {
        Effect = "Allow",
        Action = [
          "s3:PutObject",
          "s3:GetObject",
          "s3:DeleteObject",
          "s3:ListBucket",
          "s3:GetBucketLocation"
        ],
        Resource = [
          "arn:aws:s3:::${local.archive_bucket_id}",
          "arn:aws:s3:::${local.archive_bucket_id}/*"
        ]
      }
    ]
  })
}

resource "aws_iam_instance_profile" "exasol_instance_profile" {
  count = var.s3_archive_enabled ? 1 : 0
  name  = "${local.deployment_id}-instance-profile"
  role  = aws_iam_role.exasol_instance_role[0].name
  tags = {
    Name  = "${local.deployment_id}-instance-profile"
    Owner = data.aws_caller_identity.current.arn
  }
}

# Exasol nodes
resource "aws_instance" "nodes" {
  lifecycle {
    precondition {
      condition     = local.instance_vcpus >= var.min_vcpus && local.instance_ram_gb >= var.min_ram_gb
      error_message = <<-EOT
        Resource Spec Validation Failed:
        Instance Type: ${var.instance_type}
        vCPUs: ${local.instance_vcpus} (min required: ${var.min_vcpus})
        RAM: ${local.instance_ram_gb}GB (min required: ${var.min_ram_gb}GB)

        ${var.instance_type} has only ${local.instance_vcpus} vCPUs / ${local.instance_ram_gb}GB RAM. Use instance types which have at least ${var.min_vcpus} vCPUs / ${var.min_ram_gb}GB RAM or larger.
      EOT
    }
  }

  for_each = { for node in local.nodes : node.name => node }

  availability_zone = local.selected_az
  ami               = local.selected_ami_id
  instance_type     = var.instance_type
  subnet_id         = aws_subnet.subnet.id
  key_name          = local.ssh_key_name
  private_ip        = each.value.ip

  vpc_security_group_ids = [aws_security_group.exasol_instance.id]

  iam_instance_profile = var.s3_archive_enabled ? aws_iam_instance_profile.exasol_instance_profile[0].name : null

  root_block_device {
    volume_type = var.volume_type
    volume_size = var.os_volume_size
    tags = {
      Name         = "${local.deployment_id}-${each.key}-root"
      Owner        = data.aws_caller_identity.current.arn
    }
  }

  tags = {
    Name         = "${local.deployment_id}-${each.key}"
    Owner        = data.aws_caller_identity.current.arn
  }

  user_data_base64 = data.cloudinit_config.cloud_config[each.key].rendered
}

resource "aws_ebs_volume" "data_disks" {
  for_each = { for node in local.nodes : node.name => node }

  availability_zone = local.selected_az
  size              = var.data_volume_size
  type              = var.volume_type
  encrypted         = true

  tags = {
    Name         = "${local.deployment_id}-${each.key}-data"
    Node         = each.key
    Role         = "data"
    Owner        = data.aws_caller_identity.current.arn
  }
}

resource "aws_volume_attachment" "data_disks" {
  for_each = aws_ebs_volume.data_disks

  device_name = var.data_disk_device_name
  volume_id   = each.value.id
  instance_id = aws_instance.nodes[each.key].id
  # force_detach = true
  # skip_destroy = false
}

variable "power_state" {
  description = "Target power state for instances"
  type        = string
  default     = "running"
  validation {
    condition     = contains(["running", "stopped"], var.power_state)
    error_message = "Allowed values are: running, stopped"
  }
}

resource "aws_ec2_instance_state" "node_state" {
  for_each    = { for node in local.nodes : node.name => node }
  instance_id = aws_instance.nodes[each.key].id
  state       = var.power_state
}
