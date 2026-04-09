# Public variables exposed to the Exasol Personal launcher as command line interface flags
# They are shown in the order in which they are declared here

variable "vm_cpus" {
  description = "Number of CPUs in the VM"
  type        = number
  default     = 2
}

variable "vm_memory" {
  description = "Memory allocated to the VM in MiB"
  type        = number
  default     = 4096
}

variable "data_volume_size" {
  description = "Size in GB for the database data volume"
  type        = number
  default     = 30
}

variable "db_password" {
  description = "Optional database password. If empty, a random password is generated"
  type        = string
  default     = ""
  sensitive   = true
}

variable "adminui_password" {
  description = "Optional Admin UI password. If empty, a random password is generated"
  type        = string
  default     = ""
  sensitive   = true
}
