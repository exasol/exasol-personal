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
  installation_cloudconf_dir      = "${var.installation_preset_dir}/cloudconf"
  installation_files_dir          = "${var.installation_preset_dir}/files"

  # Optional infrastructure-preset-provided host file overlay.
  # This is used for infrastructure-specific scripts or configs that should not live in provider-agnostic installation presets.
  infrastructure_files_dir        = "${path.module}/files"

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
      src_path  = "${local.installation_files_dir}/${rel}"
      dest_path = "/${rel}"
      permissions = endswith(rel, ".sh") ? "0755" : "0644"
    }
  ]

  # Infrastructure preset files to be installed on the node (optional overlay).
  # These are applied after installation preset files, so infra presets can override paths when necessary.
  infrastructure_node_files = [
    for rel in fileset(local.infrastructure_files_dir, "**") : {
      src_path  = "${local.infrastructure_files_dir}/${rel}"
      dest_path = "/${rel}"
      permissions = endswith(rel, ".sh") ? "0755" : "0644"
    }
  ]

  # Cluster addressing helpers (also used for JSON payload values).
  node_ips           = [for n in local.nodes : n.ip]
  node_ips_space_sep = join(" ", local.node_ips)
  n11_ip             = one([for n in local.nodes : n.ip if n.name == "n11"])

  # JSON-friendly node list (camelCase keys).
  nodes_payload = [
    for n in local.nodes : {
      name      = n.name
      privateIp = n.ip
    }
  ]

  # Deployment metadata written to disk.
  # Keep this focused: only include information that is plausibly useful on the node
  # at runtime or for diagnostics. Operator-only settings (e.g. desired power state) should
  # not be written to the instance.
  # These values are intended for consumption on the node (scripts read them).
  #
  # NOTE: This includes sensitive material (SSH private key, DB/AdminUI passwords, TLS key).
  infrastructure_payload = {

    # Mandatory section. These values shall always be provided
    deploymentId    = local.deployment_id
    numNodes        = length(local.nodes)
    n11Ip           = local.n11_ip
        
    adminPrivateKey    = tls_private_key.ssh_key.private_key_pem
    hostAddrs          = local.node_ips_space_sep
    hostExternalAddrs  = local.node_ips_space_sep
    dbPasswordB64      = base64encode(local.db_password_final)
    adminUiPasswordB64 = base64encode(local.adminui_password_final)
    tlsKey             = tls_private_key.tls_key.private_key_pem
    tlsCa              = tls_self_signed_cert.tls_ca_cert.cert_pem
    tlsCert            = tls_locally_signed_cert.tls_cert.cert_pem
    barrierPort       = "120"    

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
      #scripts = var.s3_archive_enabled ? ["/opt/exasol_launcher/scripts/aws_registerS3ArchiveVolume.sh"] : []
      scripts = []
    }
        
    azure = {
      location     = var.location
      instanceType = var.instance_type

      image = {
        publisher = var.image_publisher
        offer     = var.image_offer
        sku       = var.image_sku
        version   = var.image_version
      }

      storage = {
        diskSku          = var.disk_sku
        osVolumeSizeGb   = var.os_volume_size
        dataVolumeSizeGb = var.data_volume_size
      }

      network = {
        vnetCidr   = var.vnet_cidr
        subnetCidr = var.subnet_cidr
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
      # Azure disk paths under /dev can vary with guest/kernel/device presentation,
      # so we store the attached LUN as the stable cloud-side identifier and let
      # prepareExasol.sh resolve it to the actual device before creating
      # /dev/exasol_data_01.
      hostDatadisk           = "/dev/exasol_data_01"
      hostDatadiskSourceType = "azure-lun"
      hostDatadiskSource     = 0
    }
  }
}

data "cloudinit_config" "cloud_config" {
  for_each = { for node in local.nodes : node.name => node }

  gzip          = false
  base64_encode = false

  dynamic "part" {
    for_each = local.installation_cloudconf_files

    content {
      content_type = "text/cloud-config"
      # Numeric prefix makes ordering/precedence explicit when debugging multipart user-data.
      filename     = "10-cloudconf-${part.value}"
      content      = file("${local.installation_cloudconf_dir}/${part.value}")
    }
  }

  part {
    content_type = "text/cloud-config"
    # Keep this last so it can intentionally override earlier preset cloud-config values.
    filename     = "99-write-files.yaml"
    content      = yamlencode({
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
}
