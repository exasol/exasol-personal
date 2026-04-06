locals {
  infrastructure_artifact_dir = abspath(var.infrastructure_artifact_dir)
  installation_preset_dir     = abspath(var.installation_preset_dir)
  key_file_name               = "node_access.pem"
  key_file_relative_path      = local.key_file_name
  key_file_path               = "${local.infrastructure_artifact_dir}/${local.key_file_name}"

  deployment_info = {
    deploymentId     = local.deployment_id
    region           = azurerm_resource_group.rg.location
    availabilityZone = ""
    clusterSize      = var.cluster_size
    clusterState     = var.power_state
    instanceType     = var.instance_type
    vpcId            = azurerm_virtual_network.vnet.id
    subnetId         = azurerm_subnet.subnet.id
    nodes = {
      for k, vm in azurerm_linux_virtual_machine.nodes :
      k => {
        publicIp         = azurerm_public_ip.nodes[k].ip_address
        privateIp        = azurerm_network_interface.nodes[k].private_ip_address
        instanceId       = vm.id
        dnsName          = azurerm_public_ip.nodes[k].fqdn
        availabilityZone = ""
        ssh = {
          username = "ubuntu"
          keyName  = local.ssh_key_name
          keyFile  = local.key_file_relative_path
          port     = "22"
          command  = "ssh -i ${local.key_file_relative_path} ubuntu@${azurerm_public_ip.nodes[k].ip_address} -p 22"
        }
        tlsCert = tls_locally_signed_cert.tls_cert.cert_pem
        database = {
          dbPort = "8563"
          uiPort = "8443"
          url    = "https://${azurerm_public_ip.nodes[k].ip_address}:8443"
        }
      }
    }
  }

  deployment_secrets = {
    adminUiUsername = "admin"
    adminUiPassword = local.adminui_password_final
    dbUsername      = "sys"
    dbPassword      = local.db_password_final
  }
}

output "deployment_info" {
  description = "Deployment information for all nodes"
  value       = local.deployment_info
}

output "deployment_secrets" {
  description = "Deployment secrets"
  value       = local.deployment_secrets
  sensitive   = true
}

output "ssh_private_key" {
  description = "The private key for SSH access"
  value       = tls_private_key.ssh_key.private_key_pem
  sensitive   = true
}

output "region" {
  description = "The Azure region where resources are deployed"
  value       = azurerm_resource_group.rg.location
}

# Save deployment_info to a local JSON file (for consumption by the Exasol Launcher or other tool)
resource "local_file" "deployment_info" {
  filename = "${local.infrastructure_artifact_dir}/deployment.json"
  content  = jsonencode(local.deployment_info)
}

# Save deployment_secrets to a local JSON file (for secure storage)
resource "local_file" "deployment_secrets" {
  filename = "${local.infrastructure_artifact_dir}/secrets.json"
  content  = jsonencode(local.deployment_secrets)
}

resource "local_file" "private_key" {
  filename        = local.key_file_path
  content         = tls_private_key.ssh_key.private_key_pem
  file_permission = "0600"
}
