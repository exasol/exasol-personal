#!/usr/bin/env bash
set -Eeuo pipefail
# installAiLab.sh - install and pre-configure the Exasol AI Lab container.
#
# This is a cloud-agnostic post-install hook (registered by the infrastructure
# preset in postInstall.scripts). It runs on the access node (n11) only, after
# the database is ready, as the unprivileged 'ubuntu' user (see
# exasol_launcher_post_install.service, User=ubuntu). It:
#   1. Runs the official exasol/ai-lab container via rootless Podman,
#      publishing the configured AI Lab port.
#   2. Seeds the AI Lab Secure Configuration Storage (SCS) with the database and
#      BucketFS connection parameters so notebooks connect with no manual setup.
#   3. Installs a Podman Quadlet unit (+ user lingering) so the container is
#      restarted on reboot and across deployment stop/start.
#
# Settings are read from infrastructure.json (the `aiLab` block) and the DB
# password from `dbPasswordB64`.

SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)

# shellcheck source=./logging.sh
source "${SCRIPT_DIR}/logging.sh"
# shellcheck source=./config.sh
source "${SCRIPT_DIR}/config.sh"

readonly AILAB_IMAGE="docker.io/exasol/ai-lab:latest"
readonly AILAB_CONTAINER="exasol-ai-lab"
readonly AILAB_VOLUME="exasol-ai-lab-notebooks"
# Jupyter always listens on 49494 inside the container.
readonly AILAB_CONTAINER_PORT=49494
# The SCS file the AI Lab notebooks load from the notebooks directory.
readonly AILAB_SCS_FILE="/home/jupyter/notebooks/ai_lab_config.db"
# The notebook-connector library lives in the JupyterLab virtualenv, not the
# system python, so the SCS must be seeded with that interpreter.
readonly AILAB_PYTHON="/home/jupyter/jupyterenv/bin/python3"

enabled="$(infra_jq -er '.aiLab.enabled // false')"
if [[ "${enabled}" != "true" ]]; then
  log_substep_info "AI Lab disabled; skipping installation"
  exit 0
fi

log_step_info "Installing Exasol AI Lab"

ai_lab_port="$(infra_jq -er '.aiLab.port')"
jupyter_password="$(infra_jq -er '.aiLab.jupyterPasswordB64' | base64 -d)"
scs_password="$(infra_jq -er '.aiLab.scsPasswordB64' | base64 -d)"
db_password="$(infra_jq -er '.dbPasswordB64' | base64 -d)"

# This script runs as the unprivileged 'ubuntu' user, so Podman runs rootless
# directly (no runuser). This hook runs from a systemd service with no user
# login session, so enable lingering first: that creates the per-user runtime
# directory (/run/user/<uid>) that rootless Podman requires. Do this before any
# Podman call so it works on a fresh boot, not only when a session already exists.
if ! sudo -n loginctl enable-linger "$(id -un)" 2>/dev/null; then
  log_substep_info "Warning: could not enable user lingering; rootless Podman may be unavailable"
fi
XDG_RUNTIME_DIR="/run/user/$(id -u)"
export XDG_RUNTIME_DIR
for _ in $(seq 1 10); do
  [[ -d "${XDG_RUNTIME_DIR}" ]] && break
  sleep 1
done

log_substep_info "Pulling AI Lab image ${AILAB_IMAGE}"
podman pull "${AILAB_IMAGE}"

# Recreate the container idempotently so re-running the hook is safe.
podman rm -f "${AILAB_CONTAINER}" >/dev/null 2>&1 || true

log_substep_info "Starting AI Lab container on port ${ai_lab_port}"
# allow_host_loopback lets the rootless container reach the database and BucketFS
# on the host (via host.containers.internal); slirp4netns blocks this by default.
# Retry the first start: on a freshly-booted host the initial rootless run right
# after the image pull can intermittently fail while the network/storage is set up.
start_attempt=0
until podman run \
  --detach \
  --name "${AILAB_CONTAINER}" \
  --restart always \
  --network slirp4netns:allow_host_loopback=true \
  --env JUPYTER_PASSWORD="${jupyter_password}" \
  --volume "${AILAB_VOLUME}:/home/jupyter/notebooks" \
  --publish "${ai_lab_port}:${AILAB_CONTAINER_PORT}" \
  "${AILAB_IMAGE}"; do
  start_attempt=$((start_attempt + 1))
  if [[ "${start_attempt}" -ge 3 ]]; then
    log_error "Failed to start AI Lab container after ${start_attempt} attempts"
    exit 1
  fi
  log_substep_info "AI Lab container start failed; retrying (attempt ${start_attempt})"
  podman rm -f "${AILAB_CONTAINER}" >/dev/null 2>&1 || true
  sleep 3
done

# Wait for the container to be running and the JupyterLab python (used for
# seeding) to be invocable before exec'ing into it.
log_substep_info "Waiting for the AI Lab container to be ready"
for _ in $(seq 1 30); do
  if podman exec --user jupyter "${AILAB_CONTAINER}" test -x "${AILAB_PYTHON}" 2>/dev/null; then
    break
  fi
  sleep 2
done

log_substep_info "Pre-seeding AI Lab connection configuration (SCS)"
# The database and BucketFS run on the host; from a rootless container the host
# is reachable via host.containers.internal. Exasol Personal uses a self-signed
# certificate, so certificate validation is disabled (cert_vld=false).
# Run as the 'jupyter' user (the image's default user is 'ubuntu'), so the SCS is
# owned by and readable from the JupyterLab environment. -i forwards the heredoc
# to python's stdin.
podman exec -i --user jupyter \
  --env SCS_PASSWORD="${scs_password}" \
  --env DB_PASSWORD="${db_password}" \
  "${AILAB_CONTAINER}" "${AILAB_PYTHON}" - "${AILAB_SCS_FILE}" <<'PY'
import os
import sys

from pathlib import Path
from exasol.nb_connector.secret_store import Secrets
from exasol.nb_connector.ai_lab_config import AILabConfig as K, StorageBackend

scs = Secrets(Path(sys.argv[1]), master_password=os.environ["SCS_PASSWORD"])

# Database connection (on-prem, external DB on the same host).
scs.save(K.storage_backend, StorageBackend.onprem.name)
scs.save(K.use_itde, "False")
scs.save(K.db_host_name, "host.containers.internal")
scs.save(K.db_port, "8563")
scs.save(K.db_user, "sys")
scs.save(K.db_password, os.environ["DB_PASSWORD"])
scs.save(K.db_encryption, "True")
scs.save(K.cert_vld, "False")

# BucketFS connection.
scs.save(K.bfs_host_name, "host.containers.internal")
scs.save(K.bfs_port, "2581")
scs.save(K.bfs_service, "bfsdefault")
scs.save(K.bfs_bucket, "default")
scs.save(K.bfs_encryption, "False")
PY

log_substep_info "Configuring AI Lab to restart on reboot"
# Lingering was enabled above. Install a Quadlet unit so systemd manages the
# container across reboots.
quadlet_dir="${HOME:-/home/ubuntu}/.config/containers/systemd"
mkdir -p "${quadlet_dir}"
cat > "${quadlet_dir}/${AILAB_CONTAINER}.container" <<QUADLET
[Container]
Image=${AILAB_IMAGE}
ContainerName=${AILAB_CONTAINER}
Network=slirp4netns:allow_host_loopback=true
Environment=JUPYTER_PASSWORD=${jupyter_password}
Volume=${AILAB_VOLUME}:/home/jupyter/notebooks
PublishPort=${ai_lab_port}:${AILAB_CONTAINER_PORT}

[Install]
WantedBy=default.target
QUADLET

log_step_info "Exasol AI Lab installation completed"
