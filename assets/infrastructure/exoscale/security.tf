# Security group for the Exasol instance
resource "exoscale_security_group" "exasol_instance" {
  name = "${local.deployment_id}-sg"
}

# Allow all internal private network traffic
resource "exoscale_security_group_rule" "internal_ingress" {
  security_group_id      = exoscale_security_group.exasol_instance.id
  type                   = "INGRESS"
  protocol               = "TCP"
  start_port             = 1
  end_port               = 65535
  user_security_group_id = exoscale_security_group.exasol_instance.id
  description            = "All internal traffic between cluster nodes"
}

resource "exoscale_security_group_rule" "internal_udp_ingress" {
  security_group_id      = exoscale_security_group.exasol_instance.id
  type                   = "INGRESS"
  protocol               = "UDP"
  start_port             = 1
  end_port               = 65535
  user_security_group_id = exoscale_security_group.exasol_instance.id
  description            = "All internal UDP traffic between cluster nodes"
}

resource "exoscale_security_group_rule" "internal_icmp_ingress" {
  security_group_id      = exoscale_security_group.exasol_instance.id
  type                   = "INGRESS"
  protocol               = "ICMP"
  icmp_type              = 8
  icmp_code              = 0
  user_security_group_id = exoscale_security_group.exasol_instance.id
  description            = "ICMP ping between cluster nodes"
}

# External access rules
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

resource "exoscale_security_group_rule" "external_ingress" {
  for_each = local.external_ingress_ports

  security_group_id = exoscale_security_group.exasol_instance.id
  type              = "INGRESS"
  protocol          = "TCP"
  start_port        = each.key
  end_port          = each.key
  cidr              = var.allowed_cidr
  description       = each.value
}
