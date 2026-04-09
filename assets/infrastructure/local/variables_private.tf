# Private variables we don't expose to the Exasol Personal launcher CLI
# If you want to make them public on Exasol Personal launcher CLI, move them to variables_public.tf
# Some of those values are just here for visiblity so their values aren't hidden deep in some source file

variable "os" {
  description = "Operating System we are running on"
  type        = string
  default     = "linux"
}

variable "arch" {
  description = "Hardware arquitecture we are running on"
  type        = string
  default     = "amd64"
}

variable "vfkit" {
  description = "Command to invoke vfkit on MacOS"
  type        = string
  default     = "vfkit"
}

variable "virtiofsd" {
  description = "Command to invoke virtiofsd on Linux"
  type        = string
  default     = "/usr/libexec/virtiofsd"
}

variable "qemu-linux" {
  description = "Command to invoke QEMU on Linux"
  type        = string
  default     = "qemu-system-x86_64"
}

variable "qemu-windows" {
  description = "Command to invoke QEMU on Windows"
  type        = string
  default     = "qemu-system-x86_64"
}
