# Public variables exposed to the Exasol Personal launcher as command line interface flags
# They are shown in the order in which they are declared here

variable "cluster_size" {
  description = "Number of nodes in the cluster (use 1 for single-node deployment)"
  type        = number
  default     = 1
}

variable "instance_type" {
  description = "Azure virtual machine type"
  type        = string
  #default     = "r6i.xlarge"  # Default to memory-optimized instance suitable for database
  default     = "Standard_E4s_v3"
}

variable "disk_sku" {
  description = "Azure managed disk SKU for OS and data disks"
  type        = string
  default     = "StandardSSD_LRS"
}

variable "os_volume_size" {
  description = "Size in GB for the OS disk"
  type        = number
  default     = 100
}

variable "data_volume_size" {
  description = "Size in GB for the database data volume"
  type        = number
  default     = 100
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

variable "s3_archive_enabled" {
  description = "Enable remote archive/backup integration: creates S3 bucket, IAM user/access keys, SSM params, and VPC S3 endpoint."
  type        = bool
  default     = true
}
