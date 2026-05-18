locals {
  deployment_id = "exasol-${var.deployment_id}"

  common_labels = {
    managed_by    = "opentofu"
    project       = "exasol-personal"
    deployment_id = local.deployment_id
  }

  # Node configuration
  node_start_num = 11
  ip_start_octet = 20 # IPs start at .20
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
resource "hcloud_network" "cluster" {
  name     = "${local.deployment_id}-network"
  ip_range = var.network_ip_range
  labels   = local.common_labels
}

resource "hcloud_network_subnet" "cluster" {
  network_id   = hcloud_network.cluster.id
  type         = "cloud"
  network_zone = "eu-central"
  ip_range     = var.subnet_ip_range
}

# Servers
resource "hcloud_server" "nodes" {
  for_each = { for node in local.nodes : node.name => node }

  name        = "${local.deployment_id}-${each.key}"
  server_type = var.server_type
  location    = var.location
  image       = var.os_image
  ssh_keys    = [hcloud_ssh_key.instance_key.id]
  labels      = merge(local.common_labels, { node = each.key })

  user_data = data.cloudinit_config.cloud_config[each.key].rendered

  firewall_ids = [hcloud_firewall.exasol_instance.id]

  network {
    network_id = hcloud_network.cluster.id
    ip         = each.value.ip
  }

  depends_on = [hcloud_network_subnet.cluster]
}

# Data volumes
resource "hcloud_volume" "data_disks" {
  for_each = { for node in local.nodes : node.name => node }

  name      = "${local.deployment_id}-${each.key}-data"
  size      = var.data_volume_size
  location  = var.location
  format    = null
  labels    = merge(local.common_labels, { node = each.key, role = "data" })
}

resource "hcloud_volume_attachment" "data_disks" {
  for_each = { for node in local.nodes : node.name => node }

  volume_id = hcloud_volume.data_disks[each.key].id
  server_id = hcloud_server.nodes[each.key].id
  automount = false
}
