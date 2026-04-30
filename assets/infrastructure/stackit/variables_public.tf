# Public variables exposed to the Exasol Personal launcher as command line interface flags
# They are shown in the order in which they are declared here

variable "region" {
  description = "StackIT region to deploy into (e.g. eu01, eu02)"
  type        = string
  default     = "eu01"
}

variable "project_id" {
  description = "UUID of the target project"
  type        = string
  default     = ""
}

variable "cluster_size" {
  description = "Number of nodes in the cluster (use 1 for single-node deployment)"
  type        = number
  default     = 1
}

variable "instance_type" {
  description = "Server machine type"
  type        = string
  default     = "m2i.4"
}

variable "volume_performance_class" {
  description = "Performance class for both OS and data volumes"
  type        = string
  default     = "storage_premium_perf6"
}

variable "os_volume_size" {
  description = "Size in GB for the OS/root volume"
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
  description = "Enable remote archive/backup integration: creates S3 bucket, IAM user/access keys, and S3 endpoint."
  type        = bool
  default     = true
}

