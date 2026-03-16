variable "location" {
  description = "Azure region for deployment"
  type        = string
  default     = ""
}

locals {
  azure_config_path  = pathexpand("~/.azure/config")
  azure_config_raw   = fileexists(local.azure_config_path) ? file(local.azure_config_path) : ""
  effective_location = coalesce(
    var.location,
    try(regex("(?m)^\\s*location\\s*=\\s*(\\S+)", local.azure_config_raw)[0], ""),
    "westeurope",
  )
}

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
