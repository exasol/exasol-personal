# Test Podman Container - Go REST Server

A simple Go REST server with a `/hello` POST endpoint that writes request bodies to files.

## Features

- Takes a directory path as a command-line argument
- Exposes a POST `/hello` endpoint
- Writes POST request body to timestamped files in the specified directory
- Responds with "hello" to each request

## Building

### Local Build

```bash
go mod download
go build -o server .
```

### Container Build

```bash
podman build -t test-server -f Containerfile .
```

### Packaged Container Build (for distribution)

To build and package the container as a distributable tar.gz file:

```bash
./build-packaged-container.sh
```

This creates a `dist/` directory containing:
- `test-server-container.tar.gz` - The packaged container image
- `USAGE.txt` - Instructions for loading and running the container
- `load-and-run.sh` - Convenience script to load the image

The entire `dist/` directory can be copied to any system with Podman installed.

## Distributing to Another System

After building the packaged container, transfer the `dist/` directory to the target system:

```bash
# On the build system, compress the dist directory
tar -czf test-server-dist.tar.gz dist/

# Transfer to target system (example using scp)
scp test-server-dist.tar.gz user@targethost:~/

# On the target system
tar -xzf test-server-dist.tar.gz
cd dist/
./load-and-run.sh
```

The target system only needs Podman installed - no Go toolchain or source code required.

## Running

### Local Run

```bash
./server -dir ./data -port 8080
```

Options:
- `-dir`: Directory to store received data (required)
- `-port`: Port to listen on (default: 8080)

### Container Run

```bash
# Create a data directory
mkdir -p data

# Run container with volume mount
podman run -p 8080:8080 -v $(pwd)/data:/data:Z test-server

# Or specify a different directory inside the container
podman run -p 8080:8080 -v $(pwd)/data:/mydata:Z test-server -dir /mydata
```

## Testing

```bash
# Send a test request
curl -X POST http://localhost:8080/hello -d "Hello, World!"

# Should respond with: hello

# Check the data directory for the created file
ls -la data/
cat data/hello-*.txt
```

## Example

```bash
# Start server
./server -dir ./data

# In another terminal
curl -X POST http://localhost:8080/hello -d "Test message 1"
# Response: hello

curl -X POST http://localhost:8080/hello -d "Test message 2"
# Response: hello

# Check files
ls data/
# hello-20260410-120000.123456.txt
# hello-20260410-120005.789012.txt
```
