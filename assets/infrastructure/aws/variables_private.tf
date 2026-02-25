# Private variables we don't expose to the Exasol Personal launcher CLI
# If you want to make them public on Exasol Personal launcher CLI, move them to variables_public.tf
# Some of those values are just here for visiblity so their values aren't hidden deep in some source file

variable "vpc_cidr" {
  description = "CIDR block for VPC"
  type        = string
  default     = "172.30.0.0/16"
}

variable "subnet_cidr" {
  description = "CIDR block for subnet"
  type        = string
  default     = "172.30.1.0/24"
}

variable "allowed_cidr" {
  description = "CIDR block allowed to access the instance (e.g., your IP address)"
  type        = string
  default     = "0.0.0.0/0"  # Warning: Should be restricted in production
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

variable "ubuntu_owner_id" {
  description = "AWS account ID for official Ubuntu AMIs (Canonical)"
  type        = string
  default     = "099720109477"
}

variable "data_disk_device_name" {
  description = "Device name used when attaching data EBS volumes to instances"
  type        = string
  default     = "/dev/sdf"
}

variable "ubuntu_version" {
  description = "Ubuntu version to use (codename)"
  type        = string
  default     = "jammy"  # Ubuntu 22.04 LTS
}

variable "ami_id" {
  description = "(Optional) AMI ID to use for the EC2 instance(s). Leave empty to auto-select latest Ubuntu. Overrides 'ubuntu_version' "
  type        = string
  default     = ""
}