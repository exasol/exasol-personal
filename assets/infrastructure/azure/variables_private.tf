variable "resource_group_name" {
  description = "Optional resource group name. If empty, a deployment-based name is generated."
  type        = string
  default     = ""
}

variable "vnet_cidr" {
  description = "CIDR block for Azure virtual network"
  type        = string
  default     = "172.30.0.0/16"
}

variable "subnet_cidr" {
  description = "CIDR block for Azure subnet"
  type        = string
  default     = "172.30.1.0/24"
}

variable "allowed_cidr" {
  description = "CIDR block allowed to access the instance (e.g., your IP address)"
  type        = string
  default     = "0.0.0.0/0"
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

variable "image_publisher" {
  description = "Azure image publisher"
  type        = string
  default     = "Canonical"
}

variable "image_offer" {
  description = "Azure image offer"
  type        = string
  default     = "0001-com-ubuntu-server-jammy"
}

variable "image_sku" {
  description = "Azure image SKU"
  type        = string
  default     = "22_04-lts-gen2"
}

variable "image_version" {
  description = "Azure image version"
  type        = string
  default     = "latest"
}
