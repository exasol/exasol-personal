locals {
  deployment_id = "exasol-${var.deployment_id}"

  # Final passwords: prefer user-provided over generated
  db_password_final      = var.db_password != "" ? var.db_password : random_password.db.result
  adminui_password_final = var.adminui_password != "" ? var.adminui_password : random_password.adminui.result

  vm_os_disk    = "vm/alpine-${var.arch}.qcow2"
  vm_efi_store  = "vm/efi-store.fd"
  vm_data_disk  = "vm/data-disk.img"
  vm_ssh_key    = "vm/vm-key"
  cloudinit_iso = "vm/cloud-init.iso"

  host_ssh_port = 22222

  installation_files_dir = "${var.installation_preset_dir}/files"
}

resource "terraform_data" "vm-darwin" {
  lifecycle {
    enabled = var.os == "darwin"
  }

  # Used in destructor because we cannot access local variables there.
  input = local.deployment_id

  provisioner "local-exec" {
    command     = <<EOF
      truncate -s 64m ${local.vm_efi_store}
      truncate -s ${var.data_volume_size}G ${local.vm_data_disk}

      # TODO: Resize OS image.

      # TODO: Check if running.
      launchctl submit -l "vfkit-${local.deployment_id}" -- \
        "${var.vfkit}" \
          --cpus "${var.vm_cpus}" \
          --memory "${var.vm_memory}" \
          --bootloader "efi,variable-store=${local.infrastructure_artifact_dir}/${local.vm_efi_store},create" \
          --disk "${local.infrastructure_artifact_dir}/${local.vm_os_disk}" \
          --disk "${local.infrastructure_artifact_dir}/${local.vm_data_disk}" \
          --net user \
          --virtio-fs "tag=hostshare,path=${local.installation_files_dir}" \
    EOF
    working_dir = local.infrastructure_artifact_dir
  }

  provisioner "local-exec" {
    when = destroy
    # TODO: Check if running.
    command = <<EOF
      launchctl remove "vfkit-${self.output}"
    EOF
  }
}

resource "terraform_data" "vm-linux" {
  lifecycle {
    enabled = var.os == "linux"
  }

  # Used in destructor because we cannot access local variables there.
  input = local.deployment_id

  provisioner "local-exec" {
    command     = <<EOF
      truncate -s ${var.data_volume_size}G ${local.vm_data_disk}

      qemu-img resize "${local.vm_os_disk}" +1G

      if ! systemctl --user is-active -q "qemu-${local.deployment_id}"; then
        systemd-run --user \
          --unit="qemu-${local.deployment_id}" \
          --property=Restart=on-failure \
          --property=WorkingDirectory="${local.infrastructure_artifact_dir}" \
          "${var.qemu-linux}" \
            -enable-kvm \
            -nographic \
            -smp "${var.vm_cpus}" \
            -m "${var.vm_memory}" \
            -hda "${local.vm_os_disk}" \
            -drive "file=${local.cloudinit_iso},format=raw,if=virtio,readonly=on" \
            -drive "file=${local.vm_data_disk},format=raw,if=virtio" \
            -nic "user,hostfwd=tcp::${local.host_ssh_port}-:22" \
            -fsdev "local,id=fsdev0,path=${local.installation_files_dir},security_model=none" \
            -device virtio-9p-pci,fsdev=fsdev0,mount_tag=hostshare
      fi
    EOF
    working_dir = local.infrastructure_artifact_dir
  }

  provisioner "local-exec" {
    when    = destroy
    command = <<EOF
      if systemctl --user is-active -q "qemu-${self.output}"; then
        systemctl --user stop "qemu-${self.output}"
        systemctl --user reset-failed "qemu-${self.output}" || true
      fi
    EOF
  }
}

resource "terraform_data" "vm-windows-hyperv" {
  lifecycle {
    enabled = var.os == "windows"
  }

  # Used in destructor because we cannot access local variables there.
  input = local.deployment_id

  provisioner "local-exec" {
    interpreter = ["PowerShell", "-Command"]

    command = <<EOF
      qemu-img resize "${local.vm_os_disk}" +1G

      $actionQemu = New-ScheduledTaskAction -Execute "${var.qemu-windows}" `
        -Argument @(
          "-accel whpx",
          "-nographic",
          "-smp ${var.vm_cpus}",
          "-m ${var.vm_memory}",
          "-hda ${local.vm_os_disk}",
          "-drive file=${local.cloudinit_iso},format=raw,if=virtio,readonly=on",
          "-drive file=${local.vm_data_disk},format=raw,if=virtio",
          "-nic user,hostfwd=tcp::${local.host_ssh_port}-:22",
          "-fsdev local,id=fsdev0,path=${local.installation_files_dir},security_model=none",
          "-device virtio-9p-pci,fsdev=fsdev0,mount_tag=hostshare"
        )

      # TODO: Check if running.
      Register-ScheduledTask `
        -TaskName "qemu-${local.deployment_id}" `
        -Action $actionQemu `
        -RunLevel Highest `
        -Force

      Start-ScheduledTask -TaskName "qemu-${local.deployment_id}"
    EOF
  }

  provisioner "local-exec" {
    when = destroy

    interpreter = ["PowerShell", "-Command"]

    # TODO: Check if running.
    command = <<EOF
      Stop-ScheduledTask -TaskName "qemu-${self.output}" -ErrorAction SilentlyContinue
      Unregister-ScheduledTask -TaskName "qemu-${self.output}" -Confir
    EOF
  }
}
