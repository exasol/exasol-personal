locals {
  deployment_id = "exasol-${var.deployment_id}"

  # Node configuration
  node_start_num = 11
  ip_start_octet = 20  # IPs start at .20 (DHCP range begins here)
  nodes = [
    for i in range(var.cluster_size) : {
      name = "n${local.node_start_num + i}"
      ip   = "172.16.0.${local.ip_start_octet + i}"
    }
  ]

  # Final passwords: prefer user-provided over generated
  db_password_final      = var.db_password != "" ? var.db_password : random_password.db.result
  adminui_password_final = var.adminui_password != "" ? var.adminui_password : random_password.adminui.result
}



# Private network for inter-node communication
# Exoscale instances get public IPs automatically — no VPC/IGW/route table needed.
resource "exoscale_private_network" "cluster" {
  zone     = var.zone
  name     = "${local.deployment_id}-network"
  start_ip = var.private_network_start_ip
  end_ip   = var.private_network_end_ip
  netmask  = var.private_network_netmask

  labels = {
    name          = "${local.deployment_id}-network"
    deployment_id = local.deployment_id
  }
}

# Archive volume SOS bucket
# Exoscale SOS is S3-compatible — we use the AWS provider with a custom endpoint.
locals {
  archive_bucket_id       = "${local.deployment_id}-sos-archive"
  sos_endpoint            = "https://sos-${var.zone}.exo.io"
}

resource "aws_s3_bucket" "remote_archive_volume" {
  provider = aws.sos
  count    = var.s3_archive_enabled ? 1 : 0
  bucket   = local.archive_bucket_id

  # Without this, a bucket that isn't empty can't be deleted by Terraform
  force_destroy = true
}

# IAM role scoped to SOS-only access for archive operations
resource "exoscale_iam_role" "archive_sos_role" {
  count       = var.s3_archive_enabled ? 1 : 0
  name        = "${local.deployment_id}-archive-sos"
  description = "Allows SOS access for Exasol archive volumes"
  editable    = true

  policy = {
    default_service_strategy = "deny"
    services = {
      sos = {
        type = "allow"
      }
    }
  }
}

# API key for instances to access the SOS bucket
resource "exoscale_iam_api_key" "archive_sos_key" {
  count   = var.s3_archive_enabled ? 1 : 0
  name    = "${local.deployment_id}-archive-sos-key"
  role_id = exoscale_iam_role.archive_sos_role[0].id
}

data "exoscale_template" "my_template" {
  zone = var.zone
  name = var.os_template
}

resource "exoscale_compute_instance" "nodes" {
  for_each = { for node in local.nodes : node.name => node }

  zone = var.zone
  name = "${local.deployment_id}-${each.key}"

  template_id = data.exoscale_template.my_template.id
  type        = var.instance_type
  disk_size   = var.os_volume_size
  ssh_keys    = [local.ssh_key_name]

  security_group_ids = [exoscale_security_group.exasol_instance.id]

  network_interface {
    network_id = exoscale_private_network.cluster.id
    ip_address = each.value.ip
  }

  state = var.power_state

  block_storage_volume_ids = [exoscale_block_storage_volume.data_disks[each.key].id]

  labels = {
    name          = "${local.deployment_id}-${each.key}"
    deployment_id = local.deployment_id
  }

  user_data = data.cloudinit_config.cloud_config[each.key].rendered
}

resource "exoscale_block_storage_volume" "data_disks" {
  for_each = { for node in local.nodes : node.name => node }

  zone = var.zone
  name = "${local.deployment_id}-${each.key}-data"
  size = var.data_volume_size

  labels = {
    name          = "${local.deployment_id}-${each.key}-data"
    node          = each.key
    role          = "data"
    deployment_id = local.deployment_id
  }
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
