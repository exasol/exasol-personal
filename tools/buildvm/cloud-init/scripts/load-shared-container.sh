#!/bin/sh
# Load and run container from shared folder based on manifest

# Ensure we run as root for proper podman access
if [ "$(id -u)" -ne 0 ]; then
  exec sudo "$0" "$@"
fi

# Don't use rootless podman - run as root to avoid cgroup issues
unset PODMAN_IGNORE_CGROUPSV1_WARNING

MANIFEST_FILE="/mnt/host/container-manifest.json"
STORED_MANIFEST="/var/lib/container-manifest.json"
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

# Constants
CONTAINER_NAME="container"
STATE_FILE="/var/lib/container-state.sha256"

# Parse mounts from manifest (will be set later if manifest exists)
VOLUME_FLAGS=""

# Check if manifest exists, fall back to stored manifest if available
SKIP_LOAD=false
if [ ! -f "$MANIFEST_FILE" ]; then
  if [ -f "$STORED_MANIFEST" ]; then
    log_msg "No manifest in shared folder, using stored manifest from previous load"
    MANIFEST_FILE="$STORED_MANIFEST"
    SKIP_LOAD=true  # Don't try to reload container, just restart it
  else
    log_msg "No manifest file found (checked shared and stored locations)"
    log_msg "Will attempt to start existing container if available"
    SKIP_LOAD=true
  fi
else
  log_msg "Reading configuration from shared folder manifest"
fi

# Check if manifest exists, fall back to stored manifest if available
HAS_MANIFEST=false
if [ ! -f "$MANIFEST_FILE" ]; then
  if [ -f "$STORED_MANIFEST" ]; then
    log_msg "No manifest in shared folder, using stored manifest from previous load"
    MANIFEST_FILE="$STORED_MANIFEST"
    HAS_MANIFEST=true
  else
    log_msg "No manifest file found (checked shared and stored locations)"
    log_msg "Will attempt to start existing container if available"
    HAS_MANIFEST=false
  fi
else
  log_msg "Reading configuration from shared folder manifest"
  HAS_MANIFEST=true
fi

# Parse configuration from manifest if we have one
if [ "$HAS_MANIFEST" = "true" ]; then
  # Extract configuration from manifest using jq
  CONTAINER_FILE=$(jq -r '.containerFile' "$MANIFEST_FILE" 2>/dev/null)
  PORT=$(jq -r '.port' "$MANIFEST_FILE" 2>/dev/null)
  ARGS=$(jq -r '.args[]' "$MANIFEST_FILE" 2>/dev/null | tr '\n' ' ')
  
  # Parse mounts from manifest
  MOUNT_COUNT=$(jq -r '.mounts | length // 0' "$MANIFEST_FILE" 2>/dev/null)
  
  if [ "$MOUNT_COUNT" -gt 0 ]; then
    log_msg "Found $MOUNT_COUNT mount(s) in manifest"
    # Build -v flags from mounts array
    for i in $(seq 0 $((MOUNT_COUNT - 1))); do
      HOST_PATH=$(jq -r ".mounts[$i].hostPath" "$MANIFEST_FILE")
      CONTAINER_PATH=$(jq -r ".mounts[$i].containerPath" "$MANIFEST_FILE")
      
      # Validate paths
      if echo "$HOST_PATH" | grep -q '\.\./'; then
        log_msg "Error: hostPath contains '..' which is not allowed: $HOST_PATH"
        exit 1
      fi
      
      if echo "$CONTAINER_PATH" | grep -q '\.\./'; then
        log_msg "Error: containerPath contains '..' which is not allowed: $CONTAINER_PATH"
        exit 1
      fi
      
      # Resolve host path relative to /mnt/host
      # Remove leading ./ if present
      HOST_PATH_CLEAN="${HOST_PATH#./}"
      FULL_HOST_PATH="/mnt/host/$HOST_PATH_CLEAN"
      
      # Create directory if it doesn't exist
      log_msg "Creating mount directory: $FULL_HOST_PATH"
      mkdir -p "$FULL_HOST_PATH"
      
      # Add -v flag
      VOLUME_FLAGS="$VOLUME_FLAGS -v ${FULL_HOST_PATH}:${CONTAINER_PATH}"
      log_msg "Mount: $FULL_HOST_PATH -> $CONTAINER_PATH"
    done
  else
    log_msg "No mounts specified in manifest, container will run without volume mounts"
  fi
fi

# Determine if we should try to load/reload a container
SKIP_LOAD=false
if [ "$HAS_MANIFEST" = "true" ]; then
  # Check if containerFile is specified
  if [ -z "$CONTAINER_FILE" ] || [ "$CONTAINER_FILE" = "null" ]; then
    log_msg "No containerFile specified in manifest"
    log_msg "Will attempt to start existing container if available"
    SKIP_LOAD=true
  else
    # Check if we're using stored manifest (no shared manifest)
    if [ "$MANIFEST_FILE" = "$STORED_MANIFEST" ]; then
      log_msg "Using stored manifest, will not reload container"
      SKIP_LOAD=true
    else
      # Validate port is specified
      if [ -z "$PORT" ] || [ "$PORT" = "null" ]; then
        log_msg "Error: port not specified in manifest"
        exit 1
      fi
      
      # Build path to container file
      SHARED_CONTAINER="/mnt/host/$CONTAINER_FILE"
      
      # Check if container file exists
      if [ ! -f "$SHARED_CONTAINER" ]; then
        log_msg "Container file not found: $SHARED_CONTAINER"
        log_msg "Will attempt to start existing container if available"
      SKIP_LOAD=true
    else
      log_msg "Found container: $SHARED_CONTAINER"
      log_msg "Port: $PORT"
      log_msg "Args: $ARGS"
    fi
  fi
fi

# Load container if we have a valid manifest with containerFile
if [ "$SKIP_LOAD" = "false" ]; then
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
      # Store the manifest for future restarts
      cp "/mnt/host/container-manifest.json" "$STORED_MANIFEST"
      log_msg "Stored manifest for future restarts"
    else
      log_msg "Failed to load container image"
      exit 1
    fi
  else
    log_msg "Skipping reload, container image unchanged"
  fi
fi  # End of SKIP_LOAD check

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

# Container runtime log file
CONTAINER_LOG_FILE="$LOG_DIR/container-runtime-$(date +%Y%m%d-%H%M%S).log"

# Run the container with args (as root with proper cgroup2 support)
# NOTE: No --cpus or --memory limits are set intentionally.
# This allows the container to automatically use all VM resources.
# If VM resources are increased (e.g., more CPUs or RAM), the container
# will automatically have access to them without configuration changes.
log_msg "Starting container on port $PORT..."
if podman run -d \
  --name "$CONTAINER_NAME" \
  --network host \
  $VOLUME_FLAGS \
  --log-driver k8s-file \
  --log-opt path="$CONTAINER_LOG_FILE" \
  "$IMAGE_NAME" \
  $ARGS; then
  log_msg "Container started successfully on port $PORT"
  log_msg "Container runtime logs: $CONTAINER_LOG_FILE"
  log_msg "=== Container Load Script Completed Successfully ==="
else
  log_msg "Failed to start container"
  log_msg "=== Container Load Script Failed ==="
  exit 1
fi
