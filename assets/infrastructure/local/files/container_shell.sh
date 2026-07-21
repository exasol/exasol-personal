#!/bin/sh

container_name="exasol-local-db"
if ! podman container exists "$container_name"; then
  echo "Exasol Local database container not found" >&2
  podman ps -a >&2
  exit 125
fi

rootfs=$(podman mount "$container_name") || exit $?
pid=$(podman inspect "$container_name" --format '{{.State.Pid}}') || exit $?
echo "Exasol Local database container image does not include a shell; using VM shell with container rootfs mounted as working directory."
echo "Container rootfs: $rootfs"
cd "$rootfs" || exit $?
if [ -n "$pid" ] && [ "$pid" != "0" ]; then
  exec nsenter --target "$pid" --uts --ipc --net /bin/sh
fi
exec /bin/sh
