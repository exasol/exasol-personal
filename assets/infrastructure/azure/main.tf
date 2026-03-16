locals {
  common_tags = {
    ManagedBy  = "opentofu"
    Project    = "exasol-personal"
    Deployment = local.deployment_id
    CreatedAt  = var.deployment_created_at
  }

  deployment_id = "exasol-${var.deployment_id}"
  rg_name       = var.resource_group_name != "" ? var.resource_group_name : "${local.deployment_id}-rg"

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

resource "azurerm_resource_group" "rg" {
  name     = local.rg_name
  location = local.effective_location
  tags     = local.common_tags
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
  lun                = 0
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
