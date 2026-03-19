# Cloud Init
#
# This file builds per-node cloud-init user data. The structure is intentionally split into:
#   1) static cloud-config (from the installation preset)
#   2) dynamically generated write_files entries:
#        - deployment metadata JSON (common for all nodes)
#        - node metadata JSON (specific to the current node)
#        - installation preset files (plain files; no Terraform-side templating)
#        - infrastructure preset files (optional; plain files)

locals {
  # --- Path "constants" ---
  # Changing these paths will break the deployment processes as we use them to find the installation preset contents
  installation_cloudconf_dir = "${var.installation_preset_dir}/cloudconf"
  installation_files_dir     = "${var.installation_preset_dir}/files"

  # Optional infrastructure-preset-provided host file overlay.
  # This is used for infrastructure-specific scripts or configs that should not live in provider-agnostic installation presets.
  infrastructure_files_dir = "${path.module}/files"

  # Changing these paths will break the installation processes on the hosts as they look for these files
  launcher_config_dir             = "/etc/exasol_launcher"
  infrastructure_json_target_path = "${local.launcher_config_dir}/infrastructure.json"
  node_json_target_path           = "${local.launcher_config_dir}/node.json"

  # Read the static cloud-config from the installation preset.
  # The preset may provide one or more YAML files in a flat directory. We include each file
  # as its own cloud-init "part" (stable lexicographic order) to avoid Terraform-side YAML
  # merging logic and keep the behavior easy to understand.
  installation_cloudconf_files = sort(
    fileset(local.installation_cloudconf_dir, "*")
  )

  # Materialize metadata of files to be installed.
  installation_node_files = [
    for rel in fileset(local.installation_files_dir, "**") : {
      src_path    = "${local.installation_files_dir}/${rel}"
      dest_path   = "/${rel}"
      permissions = endswith(rel, ".sh") ? "0755" : "0644"
    }
  ]

  # Infrastructure preset files to be installed on the node (optional overlay).
  # These are applied after installation preset files, so infra presets can override paths when necessary.
  infrastructure_node_files = [
    for rel in fileset(local.infrastructure_files_dir, "**") : {
      src_path    = "${local.infrastructure_files_dir}/${rel}"
      dest_path   = "/${rel}"
      permissions = endswith(rel, ".sh") ? "0755" : "0644"
    }
  ]

  # Cluster addressing helpers (also used for JSON payload values).
  node_ips           = [for n in local.nodes : n.ip]
  node_ips_space_sep = join(" ", local.node_ips)
  n11_ip             = one([for n in local.nodes : n.ip if n.name == "n11"])

  # Deployment metadata written to disk.
  # Keep this focused: only include information that is plausibly useful on the node
  # at runtime or for diagnostics. Operator-only settings (e.g. desired power state) should
  # not be written to the instance.
  # These values are intended for consumption on the node (scripts read them).
  #
  # NOTE: This includes sensitive material (SSH private key, DB/AdminUI passwords, TLS key).
  infrastructure_payload = {

    # Keep only fields currently consumed by host-side scripts.
    # This payload is delivered through cloud-init user-data and contributes
    # directly to the 32KB Exoscale user-data limit.
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

    # Optional infrastructure-specific hook scripts.
    preInstall = {
      # preInstall hooks run on *all* nodes
      root = {
        scripts = []
      }
      user = {
        scripts = []
      }
    }
    postInstall = {
      # postInstall hooks run on the *access node (n11) only*
      scripts = var.s3_archive_enabled ? ["/opt/exasol_launcher/scripts/exoscale_registerSOSArchiveVolume.sh"] : []
    }

    # Cloud-provider specific values needed by optional infra hooks.
    exoscale = {
      zone = var.zone
      archive = {
        enabled    = var.s3_archive_enabled
        bucketId   = local.archive_bucket_id
        volumeName = "default_archive"
        sosEndpoint = local.sos_endpoint
        accessKey  = var.s3_archive_enabled ? exoscale_iam_api_key.archive_sos_key[0].key : ""
        secretKey  = var.s3_archive_enabled ? exoscale_iam_api_key.archive_sos_key[0].secret : ""
      }
    }
  }

  # Per-node metadata written to disk (separate from infrastructure.json for clarity).
  node_payload_by_name = {
    for n in local.nodes : n.name => {
      name         = n.name
      privateIp    = n.ip
      myId         = n.name

      # Exasol always uses the same final disk alias across providers.
      # The udev match clause identifies the data disk so prepareExasol.sh can
      # create a persistent udev rule with the /dev/exasol_data_01 alias.
      # On Exoscale, virtio truncates UUIDs to 20 chars in ID_SERIAL.
      hostDatadisk      = "/dev/exasol_data_01"
      hostDatadiskMatch = "ENV{ID_SERIAL}==\"${substr(try(exoscale_block_storage_volume.data_disks[n.name].id, ""), 0, 20)}\""
    }
  }
}

data "cloudinit_config" "cloud_config" {
  for_each = { for node in local.nodes : node.name => node }
  # Keep payload size below Exoscale user-data limit (32KB) as presets/scripts evolve.
  # gzip+base64 ensures a compressed transport representation that Exoscale's
  # cloud-init agent handles natively.
  gzip          = true
  base64_encode = true

  dynamic "part" {
    for_each = local.installation_cloudconf_files

    content {
      content_type = "text/cloud-config"
      # Numeric prefix makes ordering/precedence explicit when debugging multipart user-data.
      filename = "10-cloudconf-${part.value}"
      content  = file("${local.installation_cloudconf_dir}/${part.value}")
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
            permissions = "0644"
            content     = jsonencode(local.infrastructure_payload)
          },
          {
            path        = local.node_json_target_path
            permissions = "0644"
            content     = jsonencode(local.node_payload_by_name[each.key])
          }
        ],
        [
          for f in local.installation_node_files : {
            path        = f.dest_path
            permissions = f.permissions
            content     = file(f.src_path)
          }
        ],
        [
          for f in local.infrastructure_node_files : {
            path        = f.dest_path
            permissions = f.permissions
            content     = file(f.src_path)
          }
        ]
      )
    })
  }

  part {
    content_type = "text/cloud-config"
    # MUST prepend because the network needs to be configured BEFORE the installation
    # preset starts
    filename = "99-exoscale-network.yaml"
    content = yamlencode({
      merge_how = [
        {
          name     = "list"
          settings = ["prepend"]
        },
        {
          name     = "dict"
          settings = ["no_replace", "recurse_list"]
        }
      ]
      runcmd = [
        "systemctl enable exoscale-private-network.service",
        "systemctl start exoscale-private-network.service"
      ]
    })
  }
}
