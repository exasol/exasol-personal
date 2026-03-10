# Public variables exposed to the Exasol Personal launcher as command line interface flags
# They are shown in the order in which they are declared here

variable "cluster_size" {
  description = "Number of nodes in the cluster (use 1 for single-node deployment)"
  type        = number
  default     = 1
}

variable "instance_type" {
  description = "Exoscale compute instance type"
  type        = string
  default     = "standard.extra-large"
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

variable "zone" {
  description = "Exoscale zone to deploy into (e.g. ch-gva-2, de-fra-1, de-muc-1, at-vie-1, at-vie-2, bg-sof-1). Set via TF_VAR_zone environment variable."
  type        = string
  default     = "ch-gva-2"
}

variable "s3_archive_enabled" {
  description = "Enable remote archive/backup integration: creates SOS bucket, IAM API key, and registers the archive volume."
  type        = bool
  default     = true
}

