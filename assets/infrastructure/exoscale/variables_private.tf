# Private variables we don't expose to the Exasol Personal launcher CLI
# If you want to make them public on Exasol Personal launcher CLI, move them to variables_public.tf
# Some of those values are just here for visiblity so their values aren't hidden deep in some source file

variable "private_network_start_ip" {
  description = "Start of the DHCP range for the Exoscale private network"
  type        = string
  default     = "172.16.0.20"
}

variable "private_network_end_ip" {
  description = "End of the DHCP range for the Exoscale private network"
  type        = string
  default     = "172.16.3.253"
}

variable "private_network_netmask" {
  description = "Netmask for the Exoscale private network"
  type        = string
  default     = "255.255.252.0"
}

variable "allowed_cidr" {
  description = "CIDR block allowed to access the instance (e.g., your IP address)"
  type        = string
  default     = "0.0.0.0/0" # Warning: Should be restricted in production
}

variable "os_template" {
  description = "Exoscale OS template name to use for compute instances"
  type        = string
  default     = "Linux Ubuntu 22.04 LTS 64-bit"
}