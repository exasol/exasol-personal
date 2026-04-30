# Security group for the Exasol instance
resource "stackit_security_group" "sec_group" {
  project_id = var.project_id
  labels     = local.common_labels
  name       = "${local.deployment_id}-sg"
  stateful   = true
}

resource "stackit_security_group_rule" "internal_tcp_ingress" {
  project_id               = var.project_id
  security_group_id        = stackit_security_group.sec_group.security_group_id
  remote_security_group_id = stackit_security_group.sec_group.security_group_id
  direction                = "ingress"
  protocol = {
    name = "tcp"
  }
  description = "All internal TCP traffic between cluster nodes"
}

resource "stackit_security_group_rule" "internal_udp_ingress" {
  project_id               = var.project_id
  security_group_id        = stackit_security_group.sec_group.security_group_id
  remote_security_group_id = stackit_security_group.sec_group.security_group_id
  direction                = "ingress"
  protocol = {
    name = "udp"
  }
  description = "All internal UDP traffic between cluster nodes"
}

resource "stackit_security_group_rule" "internal_icmp_ingress" {
  project_id               = var.project_id
  security_group_id        = stackit_security_group.sec_group.security_group_id
  remote_security_group_id = stackit_security_group.sec_group.security_group_id
  direction                = "ingress"
  protocol = {
    name = "icmp"
  }
  icmp_parameters = {
    type = 8
    code = 0
  }
  description = "ICMP ping between cluster nodes"
}

locals {
  external_ingress_ports = {
    22    = "SSH access"
    2581  = "Default bucketfs"
    8443  = "Exasol Admin UI"
    8563  = "Default Exasol database connection"
    20002 = "Exasol container ssh"
    20003 = "Exasol confd API"
  }
}

resource "stackit_security_group_rule" "external_ingress" {
  for_each = local.external_ingress_ports

  project_id        = var.project_id
  security_group_id = stackit_security_group.sec_group.security_group_id
  direction         = "ingress"
  protocol = {
    name = "tcp"
  }
  port_range = {
    min = each.key
    max = each.key
  }
  ip_range    = var.allowed_cidr
  description = each.value
}
