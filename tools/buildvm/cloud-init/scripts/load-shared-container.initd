#!/sbin/openrc-run

name="load-shared-container"
description="Load and run container from shared folder"

depend() {
  need localmount
  after networking import-shared-keys
}

start() {
  ebegin "Loading and running container from shared folder"
  /usr/local/bin/load-shared-container.sh
  eend $?
}

stop() {
  ebegin "Stopping shared container"
  podman stop container 2>/dev/null || true
  eend $?
}
