# Public variables exposed to the Exasol Personal launcher as command line interface flags
# They are shown in the order in which they are declared here

variable "location" {
  description = "Hetzner Cloud location to deploy into (e.g. fsn1, nbg1, hel1, ash, hil)"
  type        = string
  default     = "fsn1"
}

variable "cluster_size" {
  description = "Number of nodes in the cluster (use 1 for single-node deployment)"
  type        = number
  default     = 1
}

variable "server_type" {
  description = "Hetzner Cloud server type (e.g. ccx23, ccx33, ccx43, cpx32, cx33)"
  type        = string
  default     = "ccx33"
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
  description = "Enable remote archive/backup integration using Hetzner Object Storage (S3-compatible, EU locations only)"
  type        = bool
  default     = false
}
