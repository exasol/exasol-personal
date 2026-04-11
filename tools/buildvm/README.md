
# Basic usage
1. `task install-deps` installs QEMU and any other dependencies that are required for building this disk image.

2. `task init-vm` downloads the alpine linux NoCloud image and configure it with cloud-init.

3. `task start-vm` starts the (pre-initialized) vm in the background. You can then use `task connect` to ssh into it and `task startup-benchmark` to 

4. `task stop-vm` stops the vm.

5. `task package` compresses the `disk.img` file

Use `task

## Basic Requirements

The container image type must be `.img`. We cannot use the smaller `.qcow2` disk image format because the macos virtualization tool `vfkit` does not support it. For sharing directories with the host, we must use `virtiofs` for the same reason.

## Minimizing the container

shrinks the root disk image by mounting it to the host

# Shared directory

Containers receive a 

## Adding new authorized keys

The user will not have access to the ssh keys we used during container setup.

If they want to ssh into the container, they can add their keys to the `./authorized_keys/` directory of the folder that is shared with the vm. When the vm is restarted, keys in this directory will be picked up.

## Podman containers

The vm is configured to run one podman container. To install a container, move it to the shared directory and register it in the `container-manifest.json` file.

```json
{
  "containerFile": "test-server-container.tar.gz",
  "port": 8080,
  "args": ["-dir", "/data", "-port", "8080"]
}
```

The port is used for forwarding in both qemu and podman.
The args are passed to the container.


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