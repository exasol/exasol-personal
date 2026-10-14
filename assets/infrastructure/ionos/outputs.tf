locals {
  infrastructure_artifact_dir = abspath(var.infrastructure_artifact_dir)
  key_file_name               = "node_access.pem"
  key_file_path               = "${local.infrastructure_artifact_dir}/${local.key_file_name}"

  deployment_info = {
    deploymentId = local.deployment_id
    region       = local.location
    clusterSize  = var.cluster_size
    clusterState = var.power_state
    instanceType = var.instance_type
    nodes = {
      for node in local.nodes :
      node.name => {
        publicIp   = node.ip
        privateIp  = node.ip
        dnsName    = node.ip
        instanceId = ionoscloud_server.nodes[node.name].id
        ssh = {
          username = "ubuntu"
          keyName  = local.key_file_name
          keyFile  = local.key_file_path
          port     = "22"
          command  = "ssh -i ${local.key_file_path} ubuntu@${node.ip} -p 22"
        }
        tlsCert = tls_locally_signed_cert.tls_cert.cert_pem
        database = {
          dbPort = "8563"
          uiPort = "8443"
          url    = "https://${node.ip}:8443"
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

output "location" {
  description = "The IONOS Cloud location where resources are deployed"
  value       = local.location
}

resource "local_file" "deployment_info" {
  filename = "${local.infrastructure_artifact_dir}/deployment.json"
  content  = jsonencode(local.deployment_info)
}

resource "local_sensitive_file" "deployment_secrets" {
  filename        = "${local.infrastructure_artifact_dir}/secrets.json"
  content         = jsonencode(local.deployment_secrets)
  file_permission = "0600"
}

resource "local_sensitive_file" "private_key" {
  filename        = local.key_file_path
  content         = tls_private_key.ssh_key.private_key_pem
  file_permission = "0600" # Required for SSH key files
}
