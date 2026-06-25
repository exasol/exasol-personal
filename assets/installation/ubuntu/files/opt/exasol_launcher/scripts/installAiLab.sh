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
# shellcheck source=./shared_post_install.sh
source "${SCRIPT_DIR}/shared_post_install.sh" # provides C4_PATH

readonly AILAB_IMAGE="docker.io/exasol/ai-lab:latest"
readonly AILAB_CONTAINER="exasol-ai-lab"
readonly AILAB_VOLUME="exasol-ai-lab-notebooks"
# Jupyter always listens on 49494 inside the container.
readonly AILAB_CONTAINER_PORT=49494
# The SCS file the AI Lab notebooks load from the notebooks directory by default.
readonly AILAB_SCS_FILE="/home/jupyter/notebooks/ai_lab_secure_configuration_storage.sqlite"
# The notebook-connector library lives in the JupyterLab virtualenv, not the
# system python, so the SCS must be seeded with that interpreter.
readonly AILAB_PYTHON="/home/jupyter/jupyterenv/bin/python3"
# Default database schema pre-created so notebooks are ready to use without
# running the main configuration notebook.
readonly AILAB_DB_SCHEMA="AI_LAB"
# Dedicated BucketFS bucket pre-created for AI Lab, in the default BucketFS service.
readonly AILAB_BFS_BUCKET="ailab_bucket"
readonly AILAB_BFS_SERVICE="bfsdefault"
# BucketFS HTTP auth user for write access on Exasol.
readonly AILAB_BFS_USER="w"

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
# BucketFS bucket password: confd's bucket_add expects it base64-encoded, while
# the SCS / BucketFS HTTP auth needs it in plain text.
bfs_password_b64="$(infra_jq -er '.aiLab.bfsPasswordB64')"
bfs_password="$(printf '%s' "${bfs_password_b64}" | base64 -d)"

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

# Podman on Ubuntu 22.04 requires fully-qualified image names by default and
# refuses short names like "exasol/script-language-container:..." with "no
# unqualified-search registries defined". The AI Lab exaslct/SLC build uses
# short names pulled from docker.io, so configure docker.io as the default
# search registry for the ubuntu user. The user-level file takes precedence
# over /etc/containers/registries.conf without needing sudo.
log_substep_info "Configuring Podman unqualified-search registries"
mkdir -p "${HOME:-/home/ubuntu}/.config/containers"
printf 'unqualified-search-registries = ["docker.io"]\n' \
  > "${HOME:-/home/ubuntu}/.config/containers/registries.conf"

# Enable the Podman Docker-compatible API socket so tools inside the AI Lab
# container that use the Docker SDK (e.g. exaslct / Script Language Container
# export) can reach it at /var/run/docker.sock. The socket is mounted into the
# container below. Without this, the AI Lab export_as_is notebook fails with
# "No such file or directory" when the Docker client looks for the socket.
#
# This is the documented limitation of the containerized AI Lab: by default it
# "does not allow creating Script Language Containers (SLCs)" because the
# container has no Docker daemon, and the sanctioned workaround is to mount the
# daemon socket. See:
#   https://github.com/exasol/ai-lab/blob/main/doc/user_guide/docker/docker-usage.md
# The three Podman-specific steps here (this socket, the registries.conf above,
# and the DockerRegistryImageChecker patch below) adapt that Docker-oriented
# workaround to the rootless Podman runtime we use on the host.
log_substep_info "Enabling Podman Docker-compatible API socket"
XDG_RUNTIME_DIR="${XDG_RUNTIME_DIR}" systemctl --user enable --now podman.socket
for _ in $(seq 1 15); do
  [[ -S "${XDG_RUNTIME_DIR}/podman/podman.sock" ]] && break
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
  --volume "${XDG_RUNTIME_DIR}/podman/podman.sock:/var/run/docker.sock:Z" \
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

log_substep_info "Ensuring BucketFS bucket '${AILAB_BFS_BUCKET}' exists"
# Create a dedicated bucket for AI Lab in the default BucketFS service if it does
# not already exist. confd_client runs inside the COS container, reached via c4
# connect (same pattern as the archive-volume hook). bucket_add takes the
# read/write passwords in plain text (it stores them base64-encoded), so we pass
# the same plain-text password the SCS uses; params go as JSON to stay safe
# regardless of password punctuation.
PLAY_ID="$("${C4_PATH}" config -e .play.id)"
{
  printf 'BUCKET=%q\n' "${AILAB_BFS_BUCKET}"
  printf 'BFS=%q\n' "${AILAB_BFS_SERVICE}"
  printf 'PW=%q\n' "${bfs_password}"
  cat <<'CONNECTEOF'
set -Eeuo pipefail
if confd_client -c bucketfs_info -a "bucketfs_name: ${BFS}" -j \
  | jq -e --arg b "${BUCKET}" '.buckets[$b] // empty' >/dev/null 2>&1; then
  echo "Bucket ${BUCKET} already exists; leaving it unchanged"
else
  PARAMS=$(jq -nc --arg b "${BUCKET}" --arg s "${BFS}" --arg p "${PW}" \
    '{bucket_name:$b,bucketfs_name:$s,public:true,read_password:$p,write_password:$p}')
  confd_client -c bucket_add -A "${PARAMS}"
fi
CONNECTEOF
} | "${C4_PATH}" connect -s cos -i "${PLAY_ID}"

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
  --env DB_SCHEMA="${AILAB_DB_SCHEMA}" \
  --env BFS_SERVICE="${AILAB_BFS_SERVICE}" \
  --env BFS_BUCKET="${AILAB_BFS_BUCKET}" \
  --env BFS_USER="${AILAB_BFS_USER}" \
  --env BFS_PASSWORD="${bfs_password}" \
  "${AILAB_CONTAINER}" "${AILAB_PYTHON}" - "${AILAB_SCS_FILE}" <<'PY'
import os
import sys

from pathlib import Path
from exasol.nb_connector.secret_store import Secrets
from exasol.nb_connector.ai_lab_config import AILabConfig as K, StorageBackend
from exasol.nb_connector.connections import open_pyexasol_connection

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
scs.save(K.db_schema, os.environ["DB_SCHEMA"])

# BucketFS connection (default service is HTTPS on 2581, so encryption is on).
scs.save(K.bfs_host_name, "host.containers.internal")
scs.save(K.bfs_port, "2581")
scs.save(K.bfs_service, os.environ["BFS_SERVICE"])
scs.save(K.bfs_bucket, os.environ["BFS_BUCKET"])
scs.save(K.bfs_user, os.environ["BFS_USER"])
scs.save(K.bfs_password, os.environ["BFS_PASSWORD"])
scs.save(K.bfs_encryption, "True")

# Pre-create the database schema so notebooks are ready to use without running
# the main configuration notebook (this mirrors its "Create DB schema" step).
with open_pyexasol_connection(scs, compression=True) as con:
    con.execute(f'CREATE SCHEMA IF NOT EXISTS "{os.environ["DB_SCHEMA"]}"')
PY

log_substep_info "Configuring AI Lab to restart on reboot"
# Ubuntu 22.04 ships Podman 3.4.4 which predates Quadlet (4.4+). Use
# podman generate systemd to create a plain systemd user service instead.
systemd_user_dir="${HOME:-/home/ubuntu}/.config/systemd/user"
mkdir -p "${systemd_user_dir}"
podman generate systemd \
  --name "${AILAB_CONTAINER}" \
  --restart-policy always \
  > "${systemd_user_dir}/container-${AILAB_CONTAINER}.service"

log_substep_info "Activating AI Lab systemd unit"
XDG_RUNTIME_DIR="/run/user/$(id -u)" systemctl --user daemon-reload
XDG_RUNTIME_DIR="/run/user/$(id -u)" systemctl --user enable --now "container-${AILAB_CONTAINER}"

log_substep_info "Patching exasol_integration_test_docker_environment for Podman compatibility"
# DockerRegistryImageChecker.handle_log_line raises on any unknown status, but
# Podman emits "Already exists" for locally-cached layers during a registry pull
# check (real Docker does too). The unpatched code crashes the SLC export step
# in the export_as_is notebook. Return None instead of raising so the checker
# can continue and correctly determine whether the image is in the registry.
podman exec --user root "${AILAB_CONTAINER}" python3 - <<'PATCHPY'
import pathlib

path = pathlib.Path(
    "/home/jupyter/jupyterenv/lib/python3.10/site-packages"
    "/exasol_integration_test_docker_environment/lib/docker/images/create"
    "/utils/docker_registry_image_checker.py"
)
src = path.read_text()
old = '        raise Exception(f"Unexpected log line: {log_line}")'
new = '        return None  # unknown status (e.g. "Already exists") — not an error'
if old in src:
    path.write_text(src.replace(old, new))
    print("Patched docker_registry_image_checker.py")
else:
    print("Already patched or pattern not found — skipping")
PATCHPY

log_step_info "Exasol AI Lab installation completed"
