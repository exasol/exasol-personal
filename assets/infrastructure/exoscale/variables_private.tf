# Private variables we don't expose to the Exasol Personal launcher CLI
# If you want to make them public on Exasol Personal launcher CLI, move them to variables_public.tf
# Some of those values are just here for visiblity so their values aren't hidden deep in some source file

variable "vpc_subnet_ipv4_block" {
  description = "Private IPv4 block of the VPC subnet that connects the cluster nodes"
  type        = string
  default     = "172.16.0.0/22"
}

# Measured, not documented by Exoscale; multi-node clusters stall if it exceeds
# what the VPC carries. See "VPC Interface Configuration" in README.md.
variable "vpc_mtu" {
  description = "MTU of the VPC interface; the VPC carries IPv4 packets of up to 8930 bytes"
  type        = number
  default     = 8930
}

variable "allowed_cidr" {
  description = "CIDR block allowed to access the instance (e.g., your IP address)"
  type        = string
  default     = "0.0.0.0/0" # Warning: Should be restricted in production
}

variable "os_template" {
  description = "Exoscale OS template name to use for compute instances"
  type        = string
  default     = "Linux Ubuntu 24.04 LTS 64-bit"
}