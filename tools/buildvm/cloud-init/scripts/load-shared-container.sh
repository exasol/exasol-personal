#!/bin/sh
# Load and run container from shared folder based on manifest

# Suppress cgroups-v1 warning for podman
export PODMAN_IGNORE_CGROUPSV1_WARNING=1

MANIFEST_FILE="/mnt/host/container-manifest.json"
LOG_DIR="/mnt/host/logs"
LOG_FILE="$LOG_DIR/container-load-$(date +%Y%m%d-%H%M%S).log"

# Create logs directory
mkdir -p "$LOG_DIR"

# Function to log messages to both logger and file
log_msg() {
  local msg="$1"
  echo "[$(date '+%Y-%m-%d %H:%M:%S')] $msg" | tee -a "$LOG_FILE"
  logger -t load-shared-container "$msg"
}

# Redirect all output to log file while also displaying it
exec 1> >(tee -a "$LOG_FILE")
exec 2>&1

log_msg "=== Container Load Script Started ==="

# Exit if manifest doesn't exist
if [ ! -f "$MANIFEST_FILE" ]; then
  log_msg "No manifest file found at $MANIFEST_FILE"
  exit 0
fi

log_msg "Reading configuration from manifest"

# Extract configuration from manifest using jq
CONTAINER_FILE=$(jq -r '.containerFile' "$MANIFEST_FILE" 2>/dev/null)
PORT=$(jq -r '.port' "$MANIFEST_FILE" 2>/dev/null)
ARGS=$(jq -r '.args[]' "$MANIFEST_FILE" 2>/dev/null | tr '\n' ' ')

# Validate required fields
if [ -z "$CONTAINER_FILE" ] || [ "$CONTAINER_FILE" = "null" ]; then
  log_msg "Error: containerFile not specified in manifest"
  exit 1
fi

if [ -z "$PORT" ] || [ "$PORT" = "null" ]; then
  log_msg "Error: port not specified in manifest"
  exit 1
fi

# Build paths
SHARED_CONTAINER="/mnt/host/$CONTAINER_FILE"
DATA_DIR="/mnt/host/container-data"
CONTAINER_NAME="container"
STATE_FILE="/var/lib/container-state.sha256"

# Exit if container file doesn't exist
if [ ! -f "$SHARED_CONTAINER" ]; then
  log_msg "Container file not found: $SHARED_CONTAINER"
  exit 0
fi

log_msg "Found container: $SHARED_CONTAINER"
log_msg "Port: $PORT"
log_msg "Args: $ARGS"

# Calculate checksum of container file to detect changes
CURRENT_SHA=$(sha256sum "$SHARED_CONTAINER" | cut -d' ' -f1)
log_msg "Container file checksum: $CURRENT_SHA"

# Check if we've loaded this exact container before
RELOAD_NEEDED=true
if [ -f "$STATE_FILE" ]; then
  PREVIOUS_SHA=$(cat "$STATE_FILE")
  if [ "$CURRENT_SHA" = "$PREVIOUS_SHA" ]; then
    log_msg "Container file unchanged since last load"
    RELOAD_NEEDED=false
  else
    log_msg "Container file has changed, will reload"
    RELOAD_NEEDED=true
  fi
else
  log_msg "No previous state found, will load container"
  RELOAD_NEEDED=true
fi

# If container file changed, clean up old images and containers
if [ "$RELOAD_NEEDED" = "true" ]; then
  log_msg "Cleaning up old containers and images..."
  
  # Stop and remove all containers
  CONTAINERS=$(podman ps -a --format "{{.Names}}" 2>/dev/null || true)
  if [ -n "$CONTAINERS" ]; then
    for container in $CONTAINERS; do
      log_msg "Removing container: $container"
      podman stop "$container" 2>/dev/null || true
      podman rm "$container" 2>/dev/null || true
    done
  fi
  
  # Remove all images
  IMAGES=$(podman images --format "{{.ID}}" 2>/dev/null || true)
  if [ -n "$IMAGES" ]; then
    for image in $IMAGES; do
      log_msg "Removing image: $image"
      podman rmi -f "$image" 2>/dev/null || true
    done
  fi
  
  # Load the new container image
  log_msg "Loading new container image..."
  if podman load < "$SHARED_CONTAINER" 2>&1; then
    log_msg "Container image loaded successfully"
    # Save the checksum
    echo "$CURRENT_SHA" > "$STATE_FILE"
  else
    log_msg "Failed to load container image"
    exit 1
  fi
else
  log_msg "Skipping reload, container image unchanged"
fi

# Get the image name
IMAGE_NAME=$(podman images --format "{{.Repository}}:{{.Tag}}" | head -n 1)
if [ -z "$IMAGE_NAME" ]; then
  log_msg "Error: Could not determine image name"
  exit 1
fi

log_msg "Using image: $IMAGE_NAME"

# Check if container is already running
if podman ps --format "{{.Names}}" | grep -q "^${CONTAINER_NAME}$"; then
  log_msg "Container already running, nothing to do"
  exit 0
fi

# Stop and remove existing container if it exists but isn't running
if podman ps -a --format "{{.Names}}" | grep -q "^${CONTAINER_NAME}$"; then
  log_msg "Removing stopped container"
  podman rm "$CONTAINER_NAME" 2>/dev/null || true
fi

# Create data directory if it doesn't exist
mkdir -p "$DATA_DIR"

# Run the container with args
log_msg "Starting container on port $PORT..."
if podman run -d \
  --name "$CONTAINER_NAME" \
  -p "${PORT}:${PORT}" \
  -v "${DATA_DIR}:/data:Z" \
  "$IMAGE_NAME" \
  $ARGS; then
  log_msg "Container started successfully on port $PORT"
  log_msg "=== Container Load Script Completed Successfully ==="
else
  log_msg "Failed to start container"
  log_msg "=== Container Load Script Failed ==="
  exit 1
fi
