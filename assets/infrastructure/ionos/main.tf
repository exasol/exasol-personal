locals {
  deployment_id = "exasol-${var.deployment_id}"

  common_labels = {
    managedby  = "opentofu"
    project    = "exasol-personal"
    deployment = local.deployment_id
    createdat  = var.deployment_created_at
  }

  # Dedicated core servers have no named flavors, so instance_type encodes "<cores>x<ram_gb>".
  instance_type_parts = regexall("^([0-9]+)x([0-9]+)$", var.instance_type)
  machine_cores       = length(local.instance_type_parts) == 1 ? tonumber(local.instance_type_parts[0][0]) : 0
  machine_ram_gb      = length(local.instance_type_parts) == 1 ? tonumber(local.instance_type_parts[0][1]) : 0

  datacenter_id    = one(concat(ionoscloud_datacenter.dc[*].id, data.ionoscloud_datacenter.existing[*].id))
  location         = one(concat(ionoscloud_datacenter.dc[*].location, data.ionoscloud_datacenter.existing[*].location))
  cpu_architecture = one(concat(ionoscloud_datacenter.dc[*].cpu_architecture, data.ionoscloud_datacenter.existing[*].cpu_architecture))
  location_cpu = one([
    for arch in local.cpu_architecture : arch if arch.cpu_family == var.cpu_family
  ])

  # Node configuration
  node_start_num = 11
  node_names     = [for i in range(var.cluster_size) : "n${local.node_start_num + i}"]
  node_ips       = [for i in range(var.cluster_size) : ionoscloud_ipblock.ips.ips[i]]
  nodes = [
    for i in range(var.cluster_size) : {
      name = local.node_names[i]
      ip   = local.node_ips[i]
    }
  ]

  # IONOS firewall rules take a single source IP; null allows every source.
  allowed_source_ip = var.allowed_cidr == "0.0.0.0/0" ? null : trimsuffix(var.allowed_cidr, "/32")

  external_ingress_ports = {
    22    = "SSH access"
    2581  = "Default bucketfs"
    8443  = "Exasol Admin UI"
    8563  = "Default Exasol database connection"
    20002 = "Exasol container ssh"
    20003 = "Exasol confd API"
  }

  # Final passwords: prefer user-provided over generated
  db_password_final      = var.db_password != "" ? var.db_password : random_password.db.result
  adminui_password_final = var.adminui_password != "" ? var.adminui_password : random_password.adminui.result
}

resource "ionoscloud_datacenter" "dc" {
  count = var.datacenter_id == "" ? 1 : 0

  name        = local.deployment_id
  location    = var.location
  description = "Exasol Personal deployment ${local.deployment_id}"
}

data "ionoscloud_datacenter" "existing" {
  count = var.datacenter_id == "" ? 0 : 1

  id = var.datacenter_id
}

resource "ionoscloud_lan" "public" {
  lifecycle {
    precondition {
      condition     = local.machine_cores >= var.min_cores && local.machine_ram_gb >= var.min_ram_gb
      error_message = "instance_type '${var.instance_type}' must be <cores>x<ram_gb> with at least ${var.min_cores} cores and ${var.min_ram_gb} GB RAM (e.g. 4x16)."
    }
  }

  datacenter_id = local.datacenter_id
  name          = "${local.deployment_id}-lan"
  public        = true
}

# Reserved IPs keep each node's address stable across stop/start.
resource "ionoscloud_ipblock" "ips" {
  name     = "${local.deployment_id}-ips"
  location = local.location
  size     = var.cluster_size
}

resource "ionoscloud_server" "nodes" {
  for_each = { for node in local.nodes : node.name => node }

  lifecycle {
    precondition {
      condition     = local.location_cpu != null
      error_message = "cpu_family '${var.cpu_family}' is not offered in ${local.location}. Available: ${join(", ", local.cpu_architecture[*].cpu_family)}."
    }
    precondition {
      condition     = local.location_cpu == null || (local.machine_cores <= try(local.location_cpu.max_cores, 0) && local.machine_ram_gb * 1024 <= try(local.location_cpu.max_ram, 0))
      error_message = "instance_type '${var.instance_type}' exceeds the per-server limit of ${try(local.location_cpu.max_cores, 0)} cores / ${floor(try(local.location_cpu.max_ram, 0) / 1024)} GB RAM for ${var.cpu_family} in ${local.location}."
    }
    # User data and SSH keys are only applied at creation; changing them must not replace a running node.
    ignore_changes = [volume[0].user_data, ssh_keys]
  }

  name              = "${local.deployment_id}-${each.key}"
  hostname          = each.key
  datacenter_id     = local.datacenter_id
  type              = "ENTERPRISE"
  cores             = local.machine_cores
  ram               = local.machine_ram_gb * 1024
  cpu_family        = var.cpu_family
  availability_zone = "AUTO"
  image_name        = var.image_alias
  ssh_keys          = [tls_private_key.ssh_key.public_key_openssh]
  vm_state          = var.power_state == "running" ? "RUNNING" : "SHUTOFF"

  volume {
    name      = "${local.deployment_id}-${each.key}-os"
    size      = var.os_volume_size
    disk_type = var.disk_type
    bus       = "VIRTIO"
    user_data = data.cloudinit_config.cloud_config[each.key].rendered
  }

  nic {
    name            = "${local.deployment_id}-${each.key}-nic"
    lan             = ionoscloud_lan.public.id
    dhcp            = true
    ips             = [each.value.ip]
    firewall_active = true
    firewall_type   = "INGRESS"
  }

  dynamic "label" {
    for_each = local.common_labels
    content {
      key = label.key
      # IONOS label values accept only [a-z0-9].
      value = replace(lower(label.value), "/[^a-z0-9]/", "")
    }
  }
}

resource "ionoscloud_volume" "data_disks" {
  for_each = { for node in local.node_names : node => null }

  datacenter_id = local.datacenter_id
  server_id     = ionoscloud_server.nodes[each.key].id
  name          = "${local.deployment_id}-${each.key}-data"
  size          = var.data_volume_size
  disk_type     = var.disk_type
  bus           = "VIRTIO"
  licence_type  = "UNKNOWN"
}

# Inline nic firewall blocks are not read back by the provider, so every later apply would recreate them.
resource "ionoscloud_firewall" "rules" {
  for_each = merge(
    {
      for pair in setproduct(local.node_names, keys(local.external_ingress_ports)) :
      "${pair[0]}-tcp-${pair[1]}" => {
        node      = pair[0]
        name      = local.external_ingress_ports[pair[1]]
        protocol  = "TCP"
        source_ip = local.allowed_source_ip
        port      = tonumber(pair[1])
      }
    },
    {
      for pair in setproduct(local.node_names, range(var.cluster_size)) :
      "${pair[0]}-peer-${pair[1]}" => {
        node      = pair[0]
        name      = "Cluster traffic from ${local.node_names[pair[1]]}"
        protocol  = "ANY"
        source_ip = local.node_ips[pair[1]]
        port      = null
      }
    }
  )

  datacenter_id    = local.datacenter_id
  server_id        = ionoscloud_server.nodes[each.value.node].id
  nic_id           = ionoscloud_server.nodes[each.value.node].primary_nic
  name             = each.value.name
  protocol         = each.value.protocol
  source_ip        = each.value.source_ip
  port_range_start = each.value.port
  port_range_end   = each.value.port
  type             = "INGRESS"
}
