# Public variables exposed to the Exasol Personal launcher as command line interface flags

variable "location" {
  description = "IONOS Cloud location to deploy into (e.g. de/fra, de/txl); ignored when --datacenter-id is set"
  type        = string
  default     = "de/fra"
}

variable "cluster_size" {
  description = "Number of nodes in the cluster (use 1 for single-node deployment); each node gets its own server, data volume and reserved public IP"
  type        = number
  default     = 1
}

variable "instance_type" {
  description = "Dedicated core server size per node as <cores>x<ram_gb> (e.g. 4x16, 8x32); must fit the location and contract limits"
  type        = string
  default     = "4x16"
}

variable "cpu_family" {
  description = "CPU family for the servers; must be offered in the selected location (e.g. AMD_EPYC, INTEL_XEON, INTEL_ICELAKE)"
  type        = string
  default     = "AMD_EPYC"
}

variable "disk_type" {
  description = "Volume type for both OS and data volumes (SSD Standard, SSD Premium, HDD)"
  type        = string
  default     = "SSD Premium"
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

variable "datacenter_id" {
  description = "Optional ID of an existing IONOS datacenter to deploy into. If empty, a datacenter is created per deployment in --location"
  type        = string
  default     = ""
}
