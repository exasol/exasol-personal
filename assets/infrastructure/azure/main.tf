locals {
  common_tags = {
    ManagedBy  = "opentofu"
    Project    = "exasol-personal"
    Deployment = local.deployment_id
    CreatedAt  = var.deployment_created_at
    Owner      = data.azuread_user.current.user_principal_name
  }

  deployment_id = "exasol-${var.deployment_id}"
  rg_name       = var.resource_group_name != "" ? var.resource_group_name : "${local.deployment_id}-rg"
  data_disk_lun = 0

  # Node configuration
  node_start_num = 11
  nodes = [
    for i in range(var.cluster_size) : {
      name = "n${local.node_start_num + i}"
      ip   = "172.30.1.${local.node_start_num + i}"
    }
  ]

  # Final passwords: prefer user-provided over generated
  db_password_final      = var.db_password != "" ? var.db_password : random_password.db.result
  adminui_password_final = var.adminui_password != "" ? var.adminui_password : random_password.adminui.result
}

# Fetch specs of specified VM size from Azure
data "azapi_resource_list" "vm_sizes" {
  type                   = "Microsoft.Compute/locations/vmSizes@2024-11-01"
  parent_id              = "/subscriptions/${data.azurerm_subscription.current.subscription_id}/providers/Microsoft.Compute/locations/${var.location}"
  response_export_values = ["value"]
}

data "azurerm_subscription" "current" {}

locals {
  vm_sizes       = data.azapi_resource_list.vm_sizes.output.value
  selected_vm    = one([for s in local.vm_sizes : s if s.name == var.instance_type])
  instance_vcpus = local.selected_vm != null ? local.selected_vm.numberOfCores : 0
  instance_ram_gb = local.selected_vm != null ? local.selected_vm.memoryInMB / 1024 : 0
}

resource "azurerm_resource_group" "rg" {
  name     = local.rg_name
  location = var.location
  tags     = local.common_tags

  lifecycle {
    precondition {
      condition     = var.location != ""
      error_message = "Azure region is required. Set it via --location (e.g., --location westeurope)."
    }
  }
}

resource "azurerm_virtual_network" "vnet" {
  name                = "${local.deployment_id}-vnet"
  location            = azurerm_resource_group.rg.location
  resource_group_name = azurerm_resource_group.rg.name
  address_space       = [var.vnet_cidr]
  tags                = local.common_tags
}

resource "azurerm_subnet" "subnet" {
  name                 = "${local.deployment_id}-subnet"
  resource_group_name  = azurerm_resource_group.rg.name
  virtual_network_name = azurerm_virtual_network.vnet.name
  address_prefixes     = [var.subnet_cidr]
}

resource "azurerm_public_ip" "nodes" {
  for_each = { for node in local.nodes : node.name => node }

  name                = "${local.deployment_id}-${each.key}-pip"
  location            = azurerm_resource_group.rg.location
  resource_group_name = azurerm_resource_group.rg.name
  allocation_method   = "Static"
  sku                 = "Standard"
  domain_name_label   = "${local.deployment_id}-${each.key}"
  tags                = local.common_tags
}

resource "azurerm_network_interface" "nodes" {
  for_each = { for node in local.nodes : node.name => node }

  name                = "${local.deployment_id}-${each.key}-nic"
  location            = azurerm_resource_group.rg.location
  resource_group_name = azurerm_resource_group.rg.name
  tags                = local.common_tags

  ip_configuration {
    name                          = "primary"
    subnet_id                     = azurerm_subnet.subnet.id
    private_ip_address_allocation = "Static"
    private_ip_address            = each.value.ip
    public_ip_address_id          = azurerm_public_ip.nodes[each.key].id
  }
}

resource "azurerm_network_interface_security_group_association" "nodes" {
  for_each = azurerm_network_interface.nodes

  network_interface_id      = each.value.id
  network_security_group_id = azurerm_network_security_group.exasol_instance.id
}

resource "azurerm_linux_virtual_machine" "nodes" {
  for_each = { for node in local.nodes : node.name => node }

  lifecycle {
    precondition {
      condition     = local.selected_vm != null
      error_message = "Instance type ${var.instance_type} is not available in region ${var.location}. Please choose a different instance type or region."
    }
    precondition {
      condition     = local.instance_vcpus >= var.min_vcpus && local.instance_ram_gb >= var.min_ram_gb
      error_message = <<-EOT
        Resource Spec Validation Failed:
        Instance Type: ${var.instance_type}
        vCPUs: ${local.instance_vcpus} (min required: ${var.min_vcpus})
        RAM: ${local.instance_ram_gb}GB (min required: ${var.min_ram_gb}GB)

        ${var.instance_type} has only ${local.instance_vcpus} vCPUs / ${local.instance_ram_gb}GB RAM. Use instance types which have at least ${var.min_vcpus} vCPUs / ${var.min_ram_gb}GB RAM or larger.
      EOT
    }
  }

  name                = "${local.deployment_id}-${each.key}"
  location            = azurerm_resource_group.rg.location
  resource_group_name = azurerm_resource_group.rg.name
  size                = var.instance_type
  admin_username      = "ubuntu"
  network_interface_ids = [
    azurerm_network_interface.nodes[each.key].id
  ]
  disable_password_authentication = true

  tags                            = local.common_tags

  admin_ssh_key {
    username   = "ubuntu"
    public_key = tls_private_key.ssh_key.public_key_openssh
  }

  os_disk {
    name                 = "${local.deployment_id}-${each.key}-os"
    caching              = "ReadWrite"
    storage_account_type = var.disk_sku
    disk_size_gb         = var.os_volume_size
  }

  source_image_reference {
    publisher = var.image_publisher
    offer     = var.image_offer
    sku       = var.image_sku
    version   = var.image_version
  }

  custom_data = data.cloudinit_config.cloud_config[each.key].rendered
}

resource "azurerm_managed_disk" "data_disks" {
  for_each = { for node in local.nodes : node.name => node }

  name                 = "${local.deployment_id}-${each.key}-data"
  location             = azurerm_resource_group.rg.location
  resource_group_name  = azurerm_resource_group.rg.name
  storage_account_type = var.disk_sku
  create_option        = "Empty"
  disk_size_gb         = var.data_volume_size
  tags                 = local.common_tags
}

resource "azurerm_virtual_machine_data_disk_attachment" "data_disks" {
  for_each = azurerm_managed_disk.data_disks

  managed_disk_id    = each.value.id
  virtual_machine_id = azurerm_linux_virtual_machine.nodes[each.key].id
  lun                = local.data_disk_lun
  caching            = "ReadWrite"
}

resource "azapi_resource_action" "node_start" {
  for_each = var.power_state == "running" ? azurerm_linux_virtual_machine.nodes : {}

  type        = "Microsoft.Compute/virtualMachines@2024-11-01"
  resource_id = each.value.id
  action      = "start"
  method      = "POST"

  response_export_values = []

  depends_on = [
    azurerm_virtual_machine_data_disk_attachment.data_disks
  ]
}

variable "power_state" {
  description = "Target power state for virtual machines"
  type        = string
  default     = "running"

  validation {
    condition     = contains(["running", "stopped"], var.power_state)
    error_message = "Allowed values are: running, stopped"
  }
}

resource "azapi_resource_action" "node_stop" {
  for_each = var.power_state == "stopped" ? azurerm_linux_virtual_machine.nodes : {}

  type        = "Microsoft.Compute/virtualMachines@2024-11-01"
  resource_id = each.value.id
  action      = "deallocate"
  method      = "POST"

  response_export_values = []

  depends_on = [
    azurerm_virtual_machine_data_disk_attachment.data_disks
  ]
}
