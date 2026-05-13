# Firewall for the Exasol instances
resource "hcloud_firewall" "exasol_instance" {
  name   = "${local.deployment_id}-fw"
  labels = local.common_labels

  # External access rules
  rule {
    direction  = "in"
    protocol   = "tcp"
    port       = "22"
    source_ips = [var.allowed_cidr]
    description = "SSH access"
  }

  rule {
    direction  = "in"
    protocol   = "tcp"
    port       = "2581"
    source_ips = [var.allowed_cidr]
    description = "Default bucketfs"
  }

  rule {
    direction  = "in"
    protocol   = "tcp"
    port       = "8443"
    source_ips = [var.allowed_cidr]
    description = "Exasol Admin UI"
  }

  rule {
    direction  = "in"
    protocol   = "tcp"
    port       = "8563"
    source_ips = [var.allowed_cidr]
    description = "Default Exasol database connection"
  }

  rule {
    direction  = "in"
    protocol   = "tcp"
    port       = "20002"
    source_ips = [var.allowed_cidr]
    description = "Exasol container ssh"
  }

  rule {
    direction  = "in"
    protocol   = "tcp"
    port       = "20003"
    source_ips = [var.allowed_cidr]
    description = "Exasol confd API"
  }

  # Allow all internal cluster traffic via private network range
  rule {
    direction  = "in"
    protocol   = "tcp"
    port       = "any"
    source_ips = [var.network_ip_range]
    description = "All internal TCP traffic between cluster nodes"
  }

  rule {
    direction  = "in"
    protocol   = "udp"
    port       = "any"
    source_ips = [var.network_ip_range]
    description = "All internal UDP traffic between cluster nodes"
  }

  rule {
    direction  = "in"
    protocol   = "icmp"
    source_ips = [var.network_ip_range]
    description = "ICMP ping between cluster nodes"
  }

  # Allow all outbound traffic
  rule {
    direction       = "out"
    protocol        = "tcp"
    port            = "any"
    destination_ips = ["0.0.0.0/0", "::/0"]
    description     = "Allow all outbound TCP"
  }

  rule {
    direction       = "out"
    protocol        = "udp"
    port            = "any"
    destination_ips = ["0.0.0.0/0", "::/0"]
    description     = "Allow all outbound UDP"
  }

  rule {
    direction       = "out"
    protocol        = "icmp"
    destination_ips = ["0.0.0.0/0", "::/0"]
    description     = "Allow all outbound ICMP"
  }
}
