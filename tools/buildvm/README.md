
# Basic Requirements

We require a minimal linux virtual machine to distribute Exasol Nano. The vm must work on apple silicon using `vfkit`.

The container image type must be `.img`. We cannot use the smaller `.qcow2` disk image format because the macos virtualization tool `vfkit` does not support it. For sharing directories with the host, we must use `virtiofs` for the same reason.

## Possible Windows requirements

Running a VM on windows will always require admin rights. The VM can be run on Hyper-V without additional dependencies.

Hyper-V requires the disk image is converted to VHDX.

On windows we do not have access to virtiofs, instead we can mount a VHDX file as an dynamically expanding disk.
This slightly complicates adding new ssh keys, as the disk will need to be mounted to the host to access its content.

# Usage

1. `task install-deps` installs QEMU and any other dependencies that are required for building this disk image.

2. `task build` does several tasks. Once these tasks are complete, you can use `task start-vm`.
    - Downloads the base disk image
    - Extends its size, to make room for new packages
    - Configures it with cloud-init
    - Reduces its size, leaving an empty 100mb of space for logs

3. `task start-vm` starts the (pre-initialized) vm in the background. You can then use `task connect` to ssh into it and `task startup-benchmark` to 

4. `task stop-vm` stops the vm.

5. `task package` compresses the `disk.img` file

While a vm is running, you can use:

1. `task connect` to ssh into it

2. `task test` to run:
    - Test that new authorized ssh keys can be added via the shared folder
    - Test that a podman container be started and can write to the shared folder

## Minimizing the container

shrinks the root disk image by mounting it to the host

# Shared directory

Containers receive a 

## Adding new authorized keys

**Security Model**: Only SSH keys present in the `shared/authorized_keys` file will have access to the VM. All other keys are removed on startup.

When you run `task start-vm`, the VM's SSH key (`vm-key.pub`) is automatically copied to `shared/authorized_keys`. This key is used by `task connect` to access the VM.

When you run `task stop-vm`, the key is automatically removed from the shared folder.

## Podman containers

The vm is configured to run one podman container. To install a container, move it to the shared directory and register it in the `container-manifest.json` file.

Use `task prepare-container` to copy your container and manifest to the shared folder. The included test container (`test-podman-container/`) and `test-container` task are **placeholders for demonstration purposes** - replace them with your actual container.

```json
{
  "containerFile": "test-server-container.tar.gz",
  "port": 8080,
  "args": ["-dir", "/data", "-port", "8080"]
}
```

The port is used for forwarding in both qemu and podman.
The args are passed to the container.

### Container Loading Behavior

Containers are loaded:
1. **During VM build** (init-vm) - If a container is present in the shared folder during build, it's loaded into the VM image
2. **On each startup** - The VM checks the shared folder for new/updated containers

This allows the VM to:
- Work with containers even when the shared folder is empty (uses the container loaded during build)
- Automatically update to new containers when placed in the shared folder

### Container Data Tolerance

**Important**: Containers must be designed to tolerate missing or empty data folders.

The data directory (`/data` inside the container, mounted from `/mnt/host/container-data`) may:
- Be missing if the shared folder is unavailable (e.g., Hyper-V without data disk attached)
- Be empty if the user clears the shared folder

The container loading script automatically creates `/mnt/host/container-data` if it doesn't exist, but the underlying shared folder (`/mnt/host`) may not always be available.


# Debugging

## VM debugging

While `task init-vm` is running, the logs of the vm are written to `vm-init.log`

While `task start-vm` is running, the logs of the vm are written to `vm.log`

## Podman container debugging

Container loading logs are written to `shared/logs/` for debugging:

```bash
# For the logs of the container loading startup proceedure, see
less shared/logs/container-load-*.log

# For the stdout logs of the container itself, see
less shared/logs/container-runtime-*.log
```