# Private variables we don't expose to the Exasol Personal launcher CLI
# If you want to make them public on Exasol Personal launcher CLI, move them to variables_public.tf
# Some of those values are just here for visiblity so their values aren't hidden deep in some source file

variable "os" {
  description = "Operating System we are running on"
  type        = string
  default     = "linux"
}

variable "vfkit" {
  description = "Command to invoke vfkit"
  type        = string
  default     = "vfkit"
}

variable "qemu" {
  description = "Command to invoke QEMU"
  type        = string
  default     = "qemu-system-x86_64"
}
