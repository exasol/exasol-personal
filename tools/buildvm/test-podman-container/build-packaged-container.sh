#!/bin/bash
set -euo pipefail

# Configuration
IMAGE_NAME="test-server"
IMAGE_TAG="latest"
OUTPUT_DIR="dist"
ARCHIVE_NAME="test-server-container.tar.gz"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

echo_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

echo_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Check if podman is installed
if ! command -v podman &> /dev/null; then
    echo_error "podman is not installed. Please install podman first."
    exit 1
fi

# Change to script directory
cd "$(dirname "$0")"

echo_info "Starting container build and packaging process..."

# Create output directory
mkdir -p "$OUTPUT_DIR"

# Build the container image
echo_info "Building container image: ${IMAGE_NAME}:${IMAGE_TAG}..."
if podman build -t "${IMAGE_NAME}:${IMAGE_TAG}" -f Containerfile .; then
    echo_info "Container image built successfully"
else
    echo_error "Failed to build container image"
    exit 1
fi

# Save the container image to a tar archive
echo_info "Packaging container image to ${OUTPUT_DIR}/${ARCHIVE_NAME}..."
if podman save "${IMAGE_NAME}:${IMAGE_TAG}" | gzip > "${OUTPUT_DIR}/${ARCHIVE_NAME}"; then
    echo_info "Container image packaged successfully"
else
    echo_error "Failed to package container image"
    exit 1
fi

# Get file size
FILE_SIZE=$(du -h "${OUTPUT_DIR}/${ARCHIVE_NAME}" | cut -f1)
echo_info "Package size: ${FILE_SIZE}"

# Create instructions file
INSTRUCTIONS_FILE="${OUTPUT_DIR}/USAGE.txt"
cat > "$INSTRUCTIONS_FILE" << 'EOF'
=============================================================================
Test Server Container - Usage Instructions
=============================================================================

This package contains a containerized Go REST server with a /hello endpoint.

PREREQUISITES:
--------------
- Podman (or Docker) must be installed on the target system
- Minimum 100MB disk space available

LOADING THE CONTAINER:
---------------------
1. Extract this archive (if compressed)
2. Load the container image:

   podman load < test-server-container.tar.gz

   Or if uncompressed:
   podman load < test-server-container.tar

RUNNING THE CONTAINER:
---------------------
Basic usage:

   mkdir -p data
   podman run -p 8080:8080 -v $(pwd)/data:/data:Z test-server:latest

This will:
- Start the server on port 8080
- Mount ./data directory as /data inside the container
- Store all received POST data in ./data

Custom Options:
--------------
Specify a different directory inside the container:

   podman run -p 8080:8080 -v $(pwd)/mydata:/custom:Z test-server:latest -dir /custom

Change the port:

   podman run -p 9000:8080 -v $(pwd)/data:/data:Z test-server:latest

TESTING THE SERVER:
------------------
Send a POST request to the /hello endpoint:

   curl -X POST http://localhost:8080/hello -d "Hello, World!"

Expected response: hello

The request body will be saved to a timestamped file in the data directory:

   ls data/
   # Output: hello-20260410-120000.123456.txt

   cat data/hello-*.txt
   # Output: Hello, World!

STOPPING THE CONTAINER:
----------------------
Find the container ID:

   podman ps

Stop the container:

   podman stop <container-id>

Remove the container:

   podman rm <container-id>

ADDITIONAL NOTES:
----------------
- Each POST request creates a new file with format: hello-YYYYMMDD-HHMMSS.microseconds.txt
- The server logs all operations to stdout (view with: podman logs <container-id>)
- The container runs as a non-root user for security
- Port 8080 is exposed inside the container

For more information, see the README.md in the source repository.

=============================================================================
EOF

echo_info "Created usage instructions: ${OUTPUT_DIR}/USAGE.txt"

# Copy manifest file to output directory
if [ -f "container-manifest.json" ]; then
    cp container-manifest.json "${OUTPUT_DIR}/container-manifest.json"
    echo_info "Copied manifest file: ${OUTPUT_DIR}/container-manifest.json"
fi

# Create a simple load script for convenience
LOAD_SCRIPT="${OUTPUT_DIR}/load-and-run.sh"
cat > "$LOAD_SCRIPT" << 'EOF'
#!/bin/bash
set -euo pipefail

echo "Loading test-server container image..."
podman load < test-server-container.tar.gz

echo "Container loaded successfully!"
echo ""
echo "To run the container, execute:"
echo "  mkdir -p data"
echo "  podman run -p 8080:8080 -v \$(pwd)/data:/data:Z test-server:latest"
echo ""
echo "See USAGE.txt for more options and testing instructions."
EOF

chmod +x "$LOAD_SCRIPT"
echo_info "Created load script: ${OUTPUT_DIR}/load-and-run.sh"

# Print summary
echo ""
echo_info "================================"
echo_info "Package created successfully!"
echo_info "================================"
echo ""
echo_info "Distribution files:"
echo_info "  - ${OUTPUT_DIR}/${ARCHIVE_NAME} (${FILE_SIZE})"
echo_info "  - ${OUTPUT_DIR}/USAGE.txt"
echo_info "  - ${OUTPUT_DIR}/load-and-run.sh"
echo ""
echo_info "To distribute, copy the entire ${OUTPUT_DIR}/ directory to the target system."
echo_info "On the target system, users can run: ./load-and-run.sh"
echo ""
