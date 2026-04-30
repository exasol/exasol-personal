# Private variables we don't expose to the Exasol Personal launcher CLI
# If you want to make them public on Exasol Personal launcher CLI, move them to variables_public.tf
# Some of those values are just here for visiblity so their values aren't hidden deep in some source file

variable "allowed_cidr" {
  description = "CIDR block allowed to access the instance (e.g., your IP address)"
  type        = string
  default     = "0.0.0.0/0" # Warning: Should be restricted in production
}

variable "min_vcpus" {
  description = "Minimum vCPU count required for the selected instance type"
  type        = number
  default     = 4
}

variable "min_ram_gb" {
  description = "Minimum RAM in GB required for the selected instance type"
  type        = number
  default     = 16
}

variable "ubuntu_version" {
  description = "Ubuntu version to use (numeric)"
  type        = string
  default     = "22.04"
}

variable "image_id" {
  description = "(Optional) Image ID to use for the server instance(s). Leave empty to auto-select latest Ubuntu. Overrides 'ubuntu_version' "
  type        = string
  default     = ""
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
