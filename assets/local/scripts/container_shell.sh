container_name="__LOCAL_DB_CONTAINER_NAME__"
if ! podman container exists "$container_name"; then
  container_name="__LOCAL_RUNNER_COMPATIBILITY_DB_CONTAINER_NAME__"
fi
if ! podman container exists "$container_name"; then
  echo "Exasol Local database container not found" >&2
  podman ps -a >&2
  exit 125
fi

for shell_path in /bin/bash /usr/bin/bash /bin/sh /usr/bin/sh /bin/ash /usr/bin/ash; do
  if podman exec "$container_name" "$shell_path" -c 'exit 0' >/dev/null 2>&1; then
    exec podman exec -it "$container_name" "$shell_path"
  fi
done

rootfs=$(podman mount "$container_name") || exit $?
pid=$(podman inspect "$container_name" --format '{{.State.Pid}}') || exit $?
echo "Exasol Local database container image does not include a shell; using VM shell with container rootfs mounted as working directory."
echo "Container rootfs: $rootfs"
cd "$rootfs" || exit $?
if [ -n "$pid" ] && [ "$pid" != "0" ]; then
  exec nsenter --target "$pid" --uts --ipc --net /bin/sh
fi
exec /bin/sh
