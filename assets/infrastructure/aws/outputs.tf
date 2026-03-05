data "aws_instance" "nodes" {
  for_each    = aws_instance.nodes
  instance_id = each.value.id
  depends_on  = [aws_ec2_instance_state.node_state]
}

locals {
  infrastructure_artifact_dir = abspath(var.infrastructure_artifact_dir)
  key_file_name               = "node_access.pem"
  key_file_path               = "${local.infrastructure_artifact_dir}/${local.key_file_name}"

  deployment_info = {
    deploymentId     = local.deployment_id
    region           = data.aws_region.current.id
    availabilityZone = local.selected_az
    clusterSize      = var.cluster_size
    clusterState     = length(values(data.aws_instance.nodes)) > 0 ? values(data.aws_instance.nodes)[0].instance_state : "unknown"
    instanceType     = var.instance_type
    vpcId            = aws_vpc.vpc.id
    subnetId         = aws_subnet.subnet.id
    nodes = {
      for k, node in data.aws_instance.nodes :
      k => {
        publicIp         = node.public_ip
        privateIp        = node.private_ip
        instanceId       = node.id
        dnsName          = node.public_dns
        availabilityZone = local.selected_az
        ssh = {
          username = "ubuntu"
          keyName  = local.ssh_key_name
          keyFile  = local.key_file_path
          port     = "22"
          command  = "ssh -i ${local.key_file_path} ubuntu@${node.public_dns} -p 22"
        }
        tlsCert = tls_locally_signed_cert.tls_cert.cert_pem
        database = {
          dbPort = "8563"
          uiPort = "8443"
          url    = "https://${node.public_dns}:8443"
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
  description = "The AWS region where resources are deployed"
  value       = data.aws_region.current.id
}

output "selected_availability_zone" {
  description = "The availability zone selected based on instance type availability"
  value       = local.selected_az
}

output "available_zones_for_instance_type" {
  description = "All availability zones that support the chosen instance type"
  value       = data.aws_ec2_instance_type_offerings.available_zones.locations
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
