# Public variables exposed to the Exasol Personal launcher as command line interface flags
# They are shown in the order in which they are declared here

variable "cluster_size" {
  description = "Number of nodes in the cluster (use 1 for single-node deployment)"
  type        = number
  default     = 1
}
variable "instance_type" {
  description = "EC2 instance type"
  type        = string
  default     = "r6i.xlarge" # Default to memory-optimized instance suitable for database
}

variable "volume_type" {
  description = "EBS volume type for both OS and data volumes"
  type        = string
  default     = "gp3"
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
  description = "Enable remote archive/backup integration: creates S3 bucket, IAM user/access keys, SSM params, and VPC S3 endpoint."
  type        = bool
  default     = true
}

variable "with_ai_lab" {
  description = "Install the Exasol AI Lab (Jupyter) container alongside the database, pre-configured to connect to the database and BucketFS."
  type        = bool
  default     = false
}

variable "ai_lab_port" {
  description = "Port on which the AI Lab Jupyter server is exposed."
  type        = number
  default     = 49494
}
