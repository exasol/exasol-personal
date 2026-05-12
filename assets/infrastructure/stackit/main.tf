locals {
  deployment_id = "exasol-${var.deployment_id}"

  common_labels = {
    ManagedBy  = "opentofu"
    Project    = "exasol-personal"
    Deployment = local.deployment_id
    # Labels in STACKIT can only contain alphanumeric characters, "-", "_", and ".".
    CreatedAt = replace(replace(var.deployment_created_at, " ", "_"), ":", "-")
  }

  # Node configuration
  node_start_num = 11
  node_names     = [for i in range(var.cluster_size) : "n${local.node_start_num + i}"]
  node_ips       = [for node in local.node_names : stackit_network_interface.nics[node].ipv4]
  nodes = [
    for i in range(var.cluster_size) : {
      name = local.node_names[i]
      ip   = local.node_ips[i]
    }
  ]

  machine_vcpus     = data.stackit_machine_type.machine.vcpus
  machine_ram_gb    = data.stackit_machine_type.machine.ram / 1024
  selected_image_id = var.image_id != "" ? var.image_id : split(",", data.stackit_image_v2.ubuntu.id)[2]

  # Final passwords: prefer user-provided over generated
  db_password_final      = var.db_password != "" ? var.db_password : random_password.db.result
  adminui_password_final = var.adminui_password != "" ? var.adminui_password : random_password.adminui.result
}

# Find latest Ubuntu image
data "stackit_image_v2" "ubuntu" {
  project_id = var.project_id
  filter = {
    distro  = "ubuntu"
    version = var.ubuntu_version
  }
}

data "stackit_machine_type" "machine" {
  project_id = var.project_id
  filter     = "name == '${var.instance_type}' && vcpus >= ${var.min_vcpus} && ram >= ${var.min_ram_gb * 1024}"
}

resource "stackit_network" "network" {
  project_id = var.project_id
  labels     = local.common_labels
  name       = "${local.deployment_id}-network"
  routed     = true
  dhcp       = true
}

resource "stackit_network_interface" "nics" {
  for_each = { for node in local.node_names : node => null }

  project_id         = var.project_id
  labels             = local.common_labels
  name               = "${local.deployment_id}-${each.key}-nic"
  network_id         = stackit_network.network.network_id
  security_group_ids = [stackit_security_group.sec_group.security_group_id]
}

resource "stackit_public_ip" "public_ips" {
  for_each = { for node in local.node_names : node => null }

  project_id           = var.project_id
  labels               = local.common_labels
  network_interface_id = stackit_network_interface.nics[each.key].network_interface_id
}

resource "stackit_server" "nodes" {
  lifecycle {
    precondition {
      condition     = local.machine_vcpus >= var.min_vcpus && local.machine_ram_gb >= var.min_ram_gb
      error_message = <<-EOT
        Resource Spec Validation Failed:
        Machine Type: ${var.instance_type}
        vCPUs: ${local.machine_vcpus} (min required: ${var.min_vcpus})
        RAM: ${local.machine_ram_gb}GB (min required: ${var.min_ram_gb}GB)

        ${var.instance_type} has only ${local.machine_vcpus} vCPUs / ${local.machine_ram_gb}GB RAM. Use machine types which have at least ${var.min_vcpus} vCPUs / ${var.min_ram_gb}GB RAM or larger.
      EOT
    }
  }

  for_each = { for node in local.node_names : node => null }

  project_id     = var.project_id
  labels         = local.common_labels
  desired_status = var.power_state == "running" ? "active" : "inactive"
  boot_volume = {
    size                  = var.os_volume_size
    performance_class     = var.volume_performance_class
    source_type           = "image"
    source_id             = local.selected_image_id
    delete_on_termination = true
  }
  name         = "${local.deployment_id}-${each.key}"
  machine_type = var.instance_type
  keypair_name = stackit_key_pair.machine_key.name
  user_data    = data.cloudinit_config.cloud_config[each.key].rendered
  network_interfaces = [
    stackit_network_interface.nics[each.key].network_interface_id
  ]
}

resource "stackit_volume" "data_disks" {
  for_each = { for node in local.node_names : node => null }

  project_id        = var.project_id
  labels            = local.common_labels
  size              = var.data_volume_size
  performance_class = var.volume_performance_class
  name              = "${local.deployment_id}-${each.key}-data"
  # Using metro AZ to distribute across AZs automatically.
  availability_zone = "${var.region}-m"
}

resource "stackit_server_volume_attach" "attach_volume" {
  for_each = { for node in local.node_names : node => null }

  project_id = var.project_id
  server_id  = stackit_server.nodes[each.key].server_id
  volume_id  = stackit_volume.data_disks[each.key].volume_id
}

# Archive volume S3 bucket
# We use a UUID as the S3 bucket name as it must be globally unique
# If a user needs to find the bucket, they can use the tag containing the deployment ID
resource "random_uuid" "archive_bucket_uuid" {}
resource "random_uuid" "bootstrap_bucket_uuid" {}

locals {
  archive_bucket_name   = "${local.deployment_id}-s3-archive"
  archive_bucket_id     = "${local.archive_bucket_name}-${random_uuid.archive_bucket_uuid.result}"
  bootstrap_bucket_name = "${local.deployment_id}-boostrap"
  bootstrap_bucket_id   = "${local.bootstrap_bucket_name}-${random_uuid.bootstrap_bucket_uuid.result}"
  bootstrap_source_cidrs = sort([
    for ip in stackit_public_ip.public_ips : "${ip.ip}/32"
  ])
}

resource "stackit_objectstorage_credentials_group" "credentials_group" {
  count = var.s3_archive_enabled ? 1 : 0

  project_id = var.project_id
  name       = "${local.deployment_id}-cg"
}

resource "stackit_objectstorage_credential" "credential" {
  count = var.s3_archive_enabled ? 1 : 0

  project_id           = var.project_id
  credentials_group_id = stackit_objectstorage_credentials_group.credentials_group[0].credentials_group_id
}

resource "stackit_objectstorage_bucket" "remote_archive" {
  count      = var.s3_archive_enabled ? 1 : 0
  depends_on = [stackit_objectstorage_credentials_group.credentials_group]

  project_id = var.project_id
  name       = local.archive_bucket_id
}

resource "stackit_objectstorage_credentials_group" "bootstrap_assets" {
  project_id = var.project_id
  name       = "${local.deployment_id}-bootstrap-cg"
}

resource "stackit_objectstorage_credential" "bootstrap_assets" {
  project_id           = var.project_id
  credentials_group_id = stackit_objectstorage_credentials_group.bootstrap_assets.credentials_group_id
}

resource "minio_s3_bucket" "bootstrap_assets" {
  provider = minio.bootstrap

  bucket        = local.bootstrap_bucket_id
  acl           = "private"
  force_destroy = true
}

resource "minio_s3_bucket_anonymous_access" "bootstrap_assets" {
  provider = minio.bootstrap

  bucket = minio_s3_bucket.bootstrap_assets.bucket
  policy = jsonencode({
    Statement = [
      {
        Sid       = "AllowBootstrapReadFromDeploymentIps"
        Effect    = "Allow"
        Principal = "*"
        Action    = "s3:GetObject"
        Resource  = "urn:sgws:s3:::${minio_s3_bucket.bootstrap_assets.bucket}/*"
        Condition = {
          IpAddress = {
            "aws:SourceIp" = local.bootstrap_source_cidrs
          }
        }
      }
    ]
  })
}

resource "minio_s3_object" "bootstrap_assets" {
  provider = minio.bootstrap

  for_each = local.bootstrap_node_files_by_key

  bucket_name  = minio_s3_bucket.bootstrap_assets.bucket
  object_name  = each.key
  source       = each.value.src_path
  etag         = filemd5(each.value.src_path)
  content_type = "text/plain"
}
