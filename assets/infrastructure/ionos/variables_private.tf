# Private variables we don't expose to the Exasol Personal launcher CLI

variable "allowed_cidr" {
  description = "Source allowed to reach the public ports: 0.0.0.0/0 or a single IPv4 address as /32"
  type        = string
  default     = "0.0.0.0/0" # Warning: Should be restricted in production

  validation {
    condition     = var.allowed_cidr == "0.0.0.0/0" || can(regex("^(\\d{1,3}\\.){3}\\d{1,3}/32$", var.allowed_cidr))
    error_message = "IONOS firewall rules match a single source IP: use 0.0.0.0/0 or <ipv4>/32."
  }
}

variable "min_cores" {
  description = "Minimum core count required for the selected instance type"
  type        = number
  default     = 2
}

variable "min_ram_gb" {
  description = "Minimum RAM in GB required for the selected instance type"
  type        = number
  default     = 8
}

variable "image_alias" {
  description = "IONOS image alias for the server OS"
  type        = string
  default     = "ubuntu:24.04"
}

variable "power_state" {
  description = "Target power state for virtual machines"
  type        = string
  default     = "running"

  validation {
    condition     = contains(["running", "stopped"], var.power_state)
    error_message = "Allowed values are: running, stopped"
  }
}
