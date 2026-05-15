# Private variables we don't expose to the Exasol Personal launcher CLI
# If you want to make them public on Exasol Personal launcher CLI, move them to variables_public.tf
# Some of those values are just here for visibility so their values aren't hidden deep in some source file

variable "network_ip_range" {
  description = "IP range for the Hetzner Cloud private network (CIDR notation)"
  type        = string
  default     = "172.16.0.0/22"
}

variable "subnet_ip_range" {
  description = "IP range for the subnet within the private network (CIDR notation)"
  type        = string
  default     = "172.16.0.0/24"
}

variable "allowed_cidr" {
  description = "CIDR block allowed to access the instance (e.g., your IP address)"
  type        = string
  default     = "0.0.0.0/0" # Warning: Should be restricted in production
}

variable "os_image" {
  description = "Hetzner Cloud OS image to use for servers"
  type        = string
  default     = "ubuntu-22.04"
}

variable "power_state" {
  description = "Target power state for instances. Accepted for API compatibility; power control is handled externally via the Hetzner Cloud API."
  type        = string
  default     = "running"
  validation {
    condition     = contains(["running", "stopped"], var.power_state)
    error_message = "Allowed values are: running, stopped"
  }
}

