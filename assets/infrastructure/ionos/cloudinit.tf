# Cloud Init
#
# Per-node cloud-init user data, built from:
#   1) static cloud-config parts from the installation preset
#   2) a final write_files part with infrastructure.json, node.json, the installation
#      preset files overlay and the optional infrastructure preset files overlay

locals {
  # Changing these paths will break the deployment processes as we use them to find the installation preset contents
  installation_cloudconf_dir = "${var.installation_preset_dir}/cloudconf"
  installation_files_dir     = "${var.installation_preset_dir}/files"
  infrastructure_files_dir   = "${path.module}/files"

  # Changing these paths will break the installation processes on the hosts as they look for these files
  launcher_config_dir             = "/etc/exasol_launcher"
  infrastructure_json_target_path = "${local.launcher_config_dir}/infrastructure.json"
  node_json_target_path           = "${local.launcher_config_dir}/node.json"

  installation_cloudconf_files = sort(fileset(local.installation_cloudconf_dir, "*"))

  installation_node_files = [
    for rel in fileset(local.installation_files_dir, "**") : {
      src_path    = "${local.installation_files_dir}/${rel}"
      dest_path   = "/${rel}"
      permissions = endswith(rel, ".sh") ? "0755" : "0644"
    }
  ]

  # Applied after installation preset files, so infrastructure files win on conflicting paths.
  infrastructure_node_files = [
    for rel in fileset(local.infrastructure_files_dir, "**") : {
      src_path    = "${local.infrastructure_files_dir}/${rel}"
      dest_path   = "/${rel}"
      permissions = endswith(rel, ".sh") ? "0755" : "0644"
    }
  ]

  node_ips_space_sep = join(" ", local.node_ips)
  n11_ip             = one([for n in local.nodes : n.ip if n.name == "n11"])

  # NOTE: This includes sensitive material (SSH private key, DB/AdminUI passwords, TLS key).
  infrastructure_payload = {
    deploymentId = local.deployment_id
    numNodes     = length(local.nodes)
    n11Ip        = local.n11_ip

    adminPrivateKey    = tls_private_key.ssh_key.private_key_pem
    hostAddrs          = local.node_ips_space_sep
    hostExternalAddrs  = local.node_ips_space_sep
    dbPasswordB64      = base64encode(local.db_password_final)
    adminUiPasswordB64 = base64encode(local.adminui_password_final)
    tlsKey             = tls_private_key.tls_key.private_key_pem
    tlsCa              = tls_self_signed_cert.tls_ca_cert.cert_pem
    tlsCert            = tls_locally_signed_cert.tls_cert.cert_pem

    preInstall = {
      root = {
        scripts = []
      }
      user = {
        scripts = []
      }
    }
    postInstall = {
      scripts = []
    }
  }

  node_payload_by_name = {
    for n in local.nodes : n.name => {
      name      = n.name
      privateIp = n.ip
      myId      = n.name

      # The data volume can only be attached after the server (and its user data) exists, so its
      # UUID is unknown here; it is the second virtio disk after the boot volume.
      hostDatadisk      = "/dev/exasol_data_01"
      hostDatadiskMatch = "KERNEL==\"vdb\""
    }
  }
}

data "cloudinit_config" "cloud_config" {
  for_each = { for node in local.nodes : node.name => node }

  # IONOS expects base64 user data; gzip keeps the inline file overlays small.
  gzip          = true
  base64_encode = true

  # IONOS Ubuntu images only provision root, but the installation preset runs as ubuntu.
  part {
    content_type = "text/cloud-config"
    filename     = "00-users.yaml"
    content = yamlencode({
      users = [{
        name                = "ubuntu"
        groups              = ["adm", "sudo"]
        shell               = "/bin/bash"
        sudo                = "ALL=(ALL) NOPASSWD:ALL"
        ssh_authorized_keys = [tls_private_key.ssh_key.public_key_openssh]
      }]
    })
  }

  dynamic "part" {
    for_each = local.installation_cloudconf_files

    content {
      content_type = "text/cloud-config"
      filename     = "10-cloudconf-${part.value}"
      content      = file("${local.installation_cloudconf_dir}/${part.value}")
    }
  }

  part {
    content_type = "text/cloud-config"
    # Keep this last so it can intentionally override earlier preset cloud-config values.
    filename = "99-write-files.yaml"
    content = yamlencode({
      write_files = concat(
        [
          {
            path        = local.infrastructure_json_target_path
            permissions = "0600"
            owner       = "ubuntu:ubuntu"
            # The ubuntu user is created after non-deferred write_files runs.
            defer   = true
            content = jsonencode(local.infrastructure_payload)
          },
          {
            path        = local.node_json_target_path
            permissions = "0644"
            content     = jsonencode(local.node_payload_by_name[each.key])
          }
        ],
        [
          for f in concat(local.installation_node_files, local.infrastructure_node_files) : {
            path        = f.dest_path
            permissions = f.permissions
            encoding    = "b64"
            content     = filebase64(f.src_path)
          }
        ]
      )
    })
  }
}
