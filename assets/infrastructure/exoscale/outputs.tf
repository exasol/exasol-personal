locals {
  infrastructure_artifact_dir = abspath(var.infrastructure_artifact_dir)
  installation_preset_dir     = abspath(var.installation_preset_dir)
  key_file_name               = "node_access.pem"
  key_file_relative_path      = local.key_file_name
  key_file_path               = "${local.infrastructure_artifact_dir}/${local.key_file_name}"

  deployment_info = {
    deploymentId     = local.deployment_id
    region           = var.zone
    availabilityZone = var.zone
    clusterSize      = var.cluster_size
    clusterState     = length(exoscale_compute_instance.nodes) > 0 ? values(exoscale_compute_instance.nodes)[0].state : "unknown"
    instanceType     = var.instance_type
    vpcId            = exoscale_private_network.cluster.id
    subnetId         = exoscale_private_network.cluster.id
    nodes = {
      for k, node_config in local.nodes :
      node_config.name => {
        publicIp         = exoscale_compute_instance.nodes[node_config.name].public_ip_address
        privateIp        = node_config.ip
        instanceId       = exoscale_compute_instance.nodes[node_config.name].id
        dnsName          = exoscale_compute_instance.nodes[node_config.name].public_ip_address
        availabilityZone = var.zone
        ssh = {
          username = "ubuntu"
          keyName  = local.ssh_key_name
          keyFile  = local.key_file_relative_path
          port     = "22"
          command  = "ssh -i ${local.key_file_relative_path} ubuntu@${exoscale_compute_instance.nodes[node_config.name].public_ip_address} -p 22"
        }
        tlsCert = tls_locally_signed_cert.tls_cert.cert_pem
        database = {
          dbPort = "8563"
          uiPort = "8443"
          url    = "https://${exoscale_compute_instance.nodes[node_config.name].public_ip_address}:8443"
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

# Sensitive outputs
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
  description = "The Exoscale zone where resources are deployed"
  value       = var.zone
}

output "selected_availability_zone" {
  description = "The zone where resources are deployed"
  value       = var.zone
}

output "available_zones_for_instance_type" {
  description = "All zones that support the chosen instance type (Exoscale zones are query-free)"
  value       = [var.zone]
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

# Save the SSH private key to a local PEM file (for SSH access to the nodes)
resource "local_file" "private_key" {
  filename        = local.key_file_path
  content         = tls_private_key.ssh_key.private_key_pem
  file_permission = "0600" # Required for SSH key files
}
