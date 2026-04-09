Use `task install-deps` to install QEMU and any other dependencies that are required for building this disk image.

Use `task init-vm` to download the alpine linux NoCloud image and configure it with cloud-init.

Use `task start-vm` to run the vm in the background. You can then use `task connect` to ssh into it and `task startup-benchmark` to check
how long it takes for the vm to be accessible.

Use `task stop-vm` to stop the vm.