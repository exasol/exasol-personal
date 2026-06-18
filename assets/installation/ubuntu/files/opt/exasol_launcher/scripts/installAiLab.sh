#!/usr/bin/env bash
set -Eeuo pipefail
# installAiLab.sh - install and pre-configure the Exasol AI Lab container.
#
# This is a cloud-agnostic post-install hook (registered by the infrastructure
# preset in postInstall.scripts). It runs on the access node (n11) only, after
# the database is ready. It:
#   1. Runs the official exasol/ai-lab container via rootless Podman as the
#      'ubuntu' user, publishing the configured AI Lab port.
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

readonly AILAB_USER="ubuntu"
readonly AILAB_IMAGE="docker.io/exasol/ai-lab:latest"
readonly AILAB_CONTAINER="exasol-ai-lab"
readonly AILAB_VOLUME="exasol-ai-lab-notebooks"
# Jupyter always listens on 49494 inside the container.
readonly AILAB_CONTAINER_PORT=49494
# The SCS file the AI Lab notebooks load from the notebooks directory.
readonly AILAB_SCS_FILE="/home/jupyter/notebooks/ai_lab_config.db"

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

# Run all Podman commands rootless as the AI Lab user.
as_ailab_user() {
  runuser -u "${AILAB_USER}" -- "$@"
}

log_substep_info "Pulling AI Lab image ${AILAB_IMAGE}"
as_ailab_user podman pull "${AILAB_IMAGE}"

# Recreate the container idempotently so re-running the hook is safe.
as_ailab_user podman rm -f "${AILAB_CONTAINER}" >/dev/null 2>&1 || true

log_substep_info "Starting AI Lab container on port ${ai_lab_port}"
as_ailab_user podman run \
  --detach \
  --name "${AILAB_CONTAINER}" \
  --restart always \
  --env JUPYTER_PASSWORD="${jupyter_password}" \
  --volume "${AILAB_VOLUME}:/home/jupyter/notebooks" \
  --publish "${ai_lab_port}:${AILAB_CONTAINER_PORT}" \
  "${AILAB_IMAGE}"

log_substep_info "Pre-seeding AI Lab connection configuration (SCS)"
# The database and BucketFS run on the host; from a rootless container the host
# is reachable via host.containers.internal. Exasol Personal uses a self-signed
# certificate, so certificate validation is disabled (cert_vld=false).
as_ailab_user podman exec \
  --env SCS_PASSWORD="${scs_password}" \
  --env DB_PASSWORD="${db_password}" \
  "${AILAB_CONTAINER}" python3 - "${AILAB_SCS_FILE}" <<'PY'
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
# Enable lingering so the rootless user's services run without an active login,
# and install a Quadlet unit so systemd manages the container across reboots.
loginctl enable-linger "${AILAB_USER}" || true

quadlet_dir="/home/${AILAB_USER}/.config/containers/systemd"
install -d -o "${AILAB_USER}" -g "${AILAB_USER}" "${quadlet_dir}"
cat > "${quadlet_dir}/${AILAB_CONTAINER}.container" <<QUADLET
[Container]
Image=${AILAB_IMAGE}
ContainerName=${AILAB_CONTAINER}
Environment=JUPYTER_PASSWORD=${jupyter_password}
Volume=${AILAB_VOLUME}:/home/jupyter/notebooks
PublishPort=${ai_lab_port}:${AILAB_CONTAINER_PORT}

[Install]
WantedBy=default.target
QUADLET
chown "${AILAB_USER}:${AILAB_USER}" "${quadlet_dir}/${AILAB_CONTAINER}.container"

log_step_info "Exasol AI Lab installation completed"
