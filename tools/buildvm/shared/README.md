# Shared Folder

This folder is shared between the host and VM using virtiofs.

## Adding SSH Keys

To add additional SSH keys for VM access:

1. Create or edit `authorized_keys` file in this directory:
   ```bash
   cat ~/.ssh/id_rsa.pub >> shared/authorized_keys
   ```

2. Start the VM:
   ```bash
   task start-vm
   ```

3. The VM will automatically import keys on boot. Connect with:
   ```bash
   ssh alpine@localhost -p 2222
   ```

## Container Deployment

To automatically load and run a containerized application on VM startup:

1. Build and package the test container:
   ```bash
   task build-test-container
   ```

2. The container tarball will be automatically copied to `shared/test-server-container.tar.gz`

3. Start the VM:
   ```bash
   task start-vm
   ```

4. The VM will automatically:
   - Load the container image from the shared folder
   - Start the container on port 8080
   - Mount `shared/container-data/` for persistent data storage

5. Test the container:
   ```bash
   curl -X POST http://localhost:8080/hello -d "Hello from host!"
   ```

The container data will be stored in `shared/container-data/` and persists across VM restarts.

## Debugging

Container loading logs are written to `shared/logs/` for debugging:

```bash
# View the latest container load log
ls -lt shared/logs/container-load-*.log | head -1 | xargs cat

# Monitor logs in real-time (on VM restart)
tail -f shared/logs/container-load-*.log
```

Logs include:
- Container manifest parsing
- Image loading process
- Container startup details
- Any errors encountered

## Scripts

The `scripts/` subdirectory contains cloud-init setup scripts that are automatically copied during the `task create-cloud-init` step. These scripts are:

- `setup-system.sh` - Grows partition and mounts shared folder
- `setup-services.sh` - Configures SSH and key import service
- `import-shared-keys.sh` - Imports SSH keys from authorized_keys file
- `import-shared-keys.initd` - OpenRC service for key import
- `load-shared-container.sh` - Loads and runs container from shared folder
- `load-shared-container.initd` - OpenRC service for container loading
- `cleanup-and-shutdown.sh` - Cleans up and shuts down during build

Do not edit scripts in `shared/scripts/` directly - edit them in `cloud-init/scripts/` instead and run `task create-cloud-init` to update.
