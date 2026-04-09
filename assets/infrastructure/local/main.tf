locals {
  deployment_id = "exasol-${var.deployment_id}"

  # Final passwords: prefer user-provided over generated
  db_password_final      = var.db_password != "" ? var.db_password : random_password.db.result
  adminui_password_final = var.adminui_password != "" ? var.adminui_password : random_password.adminui.result

  vm_pid_file  = "vm.pid"
  vm_efi_store = "efi-store.fd"
  # TODO: Download the OS image.
  vm_os_disk   = "infrastructure/os.img"
  vm_data_disk = "data-disk.img"

  installation_files_dir = "${var.installation_preset_dir}/files"
}

resource "terraform_data" "vm-darwin" {
  lifecycle {
    enabled = var.os == "darwin"
  }

  # Used in destructor because we cannot access local variables there.
  input = "${local.infrastructure_artifact_dir}/${local.vm_pid_file}"

  provisioner "local-exec" {
    command     = <<EOF
      truncate -s 64m ${local.vm_efi_store} && \
      truncate -s ${var.data_volume_size}G ${local.vm_data_disk} && \
      (
        nohup ${var.vfkit} \
          --pidfile ${local.vm_pid_file} \
          --cpus ${var.vm_cpus} \
          --memory ${var.vm_memory} \
          --bootloader efi,variable-store=${local.vm_efi_store},create \
          --disk ${local.vm_os_disk} \
          --disk ${local.vm_data_disk} \
          --net user \
          --virtio-fs "tag=hostshare,path=${local.installation_files_dir}" \
          > /dev/null 2>&1 < /dev/null
      ) &
    EOF
    working_dir = local.infrastructure_artifact_dir
  }

  provisioner "local-exec" {
    when    = destroy
    command = <<EOF
      if [ -f '${self.output}' ] && ps -p $(cat '${self.output}') > /dev/null; then
        kill $(cat '${self.output}')
      fi
    EOF
  }
}

resource "terraform_data" "vm-linux" {
  lifecycle {
    enabled = var.os == "linux"
  }

  # Used in destructor because we cannot access local variables there.
  input = "${local.infrastructure_artifact_dir}/${local.vm_pid_file}"

  provisioner "local-exec" {
    command     = <<EOF
      truncate -s ${var.data_volume_size}G ${local.vm_data_disk} && \
      ${var.qemu} \
        -daemonize \
        -display none \
        -pidfile ${local.vm_pid_file} \
        -smp ${var.vm_cpus} \
        -m ${var.vm_memory} \
        -boot d \
        -cdrom ${local.vm_os_disk} \
        -drive file=${local.vm_data_disk},format=raw,index=0,media=disk \
        -netdev user,id=net0 \
        -device e1000,netdev=net0
        -fsdev "local,id=fsdev0,path=${local.installation_files_dir},security_model=none" \
        -device virtio-9p-pci,fsdev=fsdev0,mount_tag=hostshare
    EOF
    working_dir = local.infrastructure_artifact_dir
  }

  provisioner "local-exec" {
    when    = destroy
    command = <<EOF
      if [ -f '${self.output}' ] && ps -p $(cat '${self.output}') > /dev/null; then
        kill $(cat '${self.output}')
      fi
    EOF
  }
}
