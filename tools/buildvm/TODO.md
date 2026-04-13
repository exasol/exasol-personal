[ ] support port remapping in the manifest file
    - should probably be handled by the launcher script using the final vm's port mapping feature.

[ ] extend the mounting system to allow the manifest file to decide on its own disk mounts.

[ ] implement build profiles for windows and mac, making the choice of which vm image to download easy

[ ] separate initial setup shared directory used by init-vm from the runtime shared directroy used by start-vm
    - not needed except as a test

[ ] prevent vm wasting time in the boot menu

[ ] configure aggressive log rotation / move logs to shared directory

[x] the container should be loaded into the vm as part of the init-vm task

[x] increase the timeouts on init-vm and test operations

[x] if the container manifest does not list a path to the container file, the existing container should be run

[x] remove container name from the manifest

[x] document container loading system

[x] fix shrinking the partition is breaking the disk

[x] support x86

[x] packaging as .vhdx for windows hyper-v

[x] tail qemu logs automatically for the init-vm task

[x] ensure podman is using all resources available in the vm, not limited by cgroups

[x] clear all authorized keys that are not included in the shared directory