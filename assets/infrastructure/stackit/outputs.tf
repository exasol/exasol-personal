locals {
  infrastructure_artifact_dir = abspath(var.infrastructure_artifact_dir)
  key_file_name               = "node_access.pem"
  key_file_path               = "${local.infrastructure_artifact_dir}/${local.key_file_name}"

  deployment_info = {
    deploymentId = local.deployment_id
    region       = var.region
    clusterSize  = var.cluster_size
    clusterState = var.power_state
    instanceType = var.instance_type
    nodes = {
      for k, node in local.nodes :
      node.name => {
        publicIp   = stackit_public_ip.public_ips[node.name].ip
        privateIp  = stackit_network_interface.nics[node.name].ipv4
        dnsName    = stackit_public_ip.public_ips[node.name].ip
        instanceId = stackit_server.nodes[node.name].server_id
        ssh = {
          username = "ubuntu"
          keyName  = local.ssh_key_name
          keyFile  = local.key_file_path
          port     = "22"
          command  = "ssh -i ${local.key_file_path} ubuntu@${stackit_public_ip.public_ips[node.name].ip} -p 22"
        }
        tlsCert = tls_locally_signed_cert.tls_cert.cert_pem
        database = {
          dbPort = "8563"
          uiPort = "8443"
          url    = "https://${stackit_public_ip.public_ips[node.name].ip}:8443"
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
  description = "The StackIT region where resources are deployed"
  value       = var.region
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
