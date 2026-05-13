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

# Manage server power state via Hetzner Cloud API
# Uses null_resource with local-exec provisioners since hcloud provider lacks a native power state resource
resource "null_resource" "server_power" {
  for_each = { for node in local.nodes : node.name => node }

  triggers = {
    power_state = var.power_state
    server_id   = hcloud_server.nodes[each.key].id
  }

  # Stop server when power_state changes to "stopped"
  provisioner "local-exec" {
    when    = create
    command = <<-EOT
      if [ -z "$HCLOUD_TOKEN" ]; then
        echo "ERROR: HCLOUD_TOKEN is not set" >&2
        exit 1
      fi
      if [ "${var.power_state}" = "stopped" ]; then
        echo "Shutting down server ${hcloud_server.nodes[each.key].id}..."
        RESPONSE=$(curl -sf -X POST \
          -H "Authorization: Bearer $HCLOUD_TOKEN" \
          -H "Content-Type: application/json" \
          "https://api.hetzner.cloud/v1/servers/${hcloud_server.nodes[each.key].id}/actions/shutdown")
        if [ $? -ne 0 ]; then
          echo "Graceful shutdown failed, forcing power off..."
          RESPONSE=$(curl -sf -X POST \
            -H "Authorization: Bearer $HCLOUD_TOKEN" \
            -H "Content-Type: application/json" \
            "https://api.hetzner.cloud/v1/servers/${hcloud_server.nodes[each.key].id}/actions/poweroff")
          if [ $? -ne 0 ]; then
            echo "ERROR: poweroff failed" >&2
            exit 1
          fi
        fi
        ACTION_ID=$(echo "$RESPONSE" | grep -o '"id":[0-9]*' | head -1 | cut -d: -f2)
        if [ -n "$ACTION_ID" ]; then
          echo "Waiting for action $ACTION_ID to complete..."
          for i in $(seq 1 60); do
            STATUS=$(curl -sf \
              -H "Authorization: Bearer $HCLOUD_TOKEN" \
              "https://api.hetzner.cloud/v1/actions/$ACTION_ID" | grep -o '"status":"[^"]*"' | cut -d'"' -f4)
            if [ "$STATUS" = "success" ]; then
              echo "Server stopped successfully."
              break
            elif [ "$STATUS" = "error" ]; then
              echo "ERROR: action $ACTION_ID failed" >&2
              exit 1
            fi
            sleep 2
          done
        fi
      fi
    EOT
  }

  # Start server when power_state changes to "running"
  provisioner "local-exec" {
    when    = create
    command = <<-EOT
      if [ -z "$HCLOUD_TOKEN" ]; then
        echo "ERROR: HCLOUD_TOKEN is not set" >&2
        exit 1
      fi
      if [ "${var.power_state}" = "running" ]; then
        echo "Powering on server ${hcloud_server.nodes[each.key].id}..."
        RESPONSE=$(curl -sf -X POST \
          -H "Authorization: Bearer $HCLOUD_TOKEN" \
          -H "Content-Type: application/json" \
          "https://api.hetzner.cloud/v1/servers/${hcloud_server.nodes[each.key].id}/actions/poweron")
        if [ $? -ne 0 ]; then
          echo "ERROR: poweron failed" >&2
          exit 1
        fi
        ACTION_ID=$(echo "$RESPONSE" | grep -o '"id":[0-9]*' | head -1 | cut -d: -f2)
        if [ -n "$ACTION_ID" ]; then
          echo "Waiting for action $ACTION_ID to complete..."
          for i in $(seq 1 60); do
            STATUS=$(curl -sf \
              -H "Authorization: Bearer $HCLOUD_TOKEN" \
              "https://api.hetzner.cloud/v1/actions/$ACTION_ID" | grep -o '"status":"[^"]*"' | cut -d'"' -f4)
            if [ "$STATUS" = "success" ]; then
              echo "Server started successfully."
              break
            elif [ "$STATUS" = "error" ]; then
              echo "ERROR: action $ACTION_ID failed" >&2
              exit 1
            fi
            sleep 2
          done
        fi
      fi
    EOT
  }
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
