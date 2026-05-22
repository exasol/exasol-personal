# Development Guide

This guide provides detailed instructions for developers working on the Exasol Personal project.

## Prerequisites

### Required Tools

- **[Go](https://golang.org/doc/install)** - See `go.mod` for required version
- **[Python](https://www.python.org/downloads/)** - Required for integration and deployment tests
- **[Task](https://taskfile.dev/)** - Build automation tool
  ```bash
  # Install Task (recommended method)
  sh -c "$(curl --location https://taskfile.dev/install.sh)" -- -d -b ~/.local/bin
  
  # Or install from Go
  go install github.com/go-task/task/v3/cmd/task@latest
  ```
  
  Ensure the installation directory is in your `PATH`.

### Supported Platforms

Development is supported on:
- **Linux** (primary development platform)
- **macOS** (Intel and Apple Silicon)
- **Windows**

### Development Tools

The project uses the following tools (automatically managed by Go's tool dependencies):

- **[golangci-lint](https://golangci-lint.run/)** - Go linting and static analysis
- **[counterfeiter](https://github.com/maxbrunsfeld/counterfeiter)** - Mock generation for testing
- **[tflint](https://github.com/terraform-linters/tflint)** - Terraform/OpenTofu linting
- **[Poetry](https://python-poetry.org/)** - Python dependency management for tests
- **[pytest](https://pytest.org/)** - Python testing framework
- **[ruff](https://docs.astral.sh/ruff/)** - Python linting and formatting
- **[mypy](https://mypy-lang.org/)** - Python type checking

These tools are invoked via Task commands and don't need manual installation.

## Getting Started

```bash
# Clone the repository
git clone https://github.com/exasol/exasol-personal.git
cd exasol-personal

# Generate code and stage embedded assets
task generate

# Build the binary
task build

# Run it
./bin/exasol version
```

## Building

### Standard Build

```bash
# Generate code and stage platform-specific embedded assets
task generate

# Build the binary
task build

# Result: bin/exasol (or bin/exasol.exe on Windows)
```

### Cross-Compilation

Build for different platforms using Go's cross-compilation:

```bash
# Example: Build for Windows from Linux/macOS
GOOS=windows GOARCH=amd64 task build

# Example: Build for macOS Apple Silicon
GOOS=darwin GOARCH=arm64 task build
```

**Note:** The launcher resolves [OpenTofu](https://opentofu.org/) at runtime, so builds no longer need platform-specific OpenTofu downloads. For macOS Apple Silicon builds, `task generate` stages the Exasol Local runner from `RUNNER_PATH`, a cached runner, or the public Exasol Local VM release download.

```bash
GOOS=darwin GOARCH=arm64 task generate RUNNER_PATH=/path/to/launcher
```

### Building Without Task

If you prefer to use Go commands directly (or Task is unavailable):

```bash
# Generate code
go generate ./...

# For macOS Apple Silicon builds, stage the Exasol Local runner for embedding.
# This uses RUNNER_PATH, a cached runner, or the release download.
go run ./tools/localrunner stage

# Build the binary
go build -o bin/exasol ./cmd/exasol

# For cross-compilation, specify target OS and architecture
GOOS=windows GOARCH=amd64 go build -o bin/exasol.exe ./cmd/exasol
```

## Development Workflow

### Essential Task Commands

View all available tasks:
```bash
task --list
```

### Typical Development Cycle

1. **Make code changes**

2. **Generate code** (if needed):
   ```bash
   task generate
   ```
   Run this after modifying generated code or interfaces.

3. **Format code**:
   ```bash
   task fmt
   ```

4. **Run linters**:
   ```bash
   task lint
   
   # Or auto-fix some issues
   task lint-golang-fix
   ```

5. **Run tests**:
   ```bash
   # Go unit tests
   task tests-unit
   
   # Python integration tests (requires test setup)
   task tests-integration
   ```

6. **Build**:
   ```bash
   task build
   ```

7. **Test manually**:
   ```bash
   ./bin/exasol <command>
   ```

### All-in-One

Run the full pipeline:
```bash
task all    # Runs lint, test, and build
```

## Testing

The project uses a combination of Go unit tests and Python integration/deployment tests. For detailed information about test types, strategy, and usage, see the [Testing README](../tests/README.md).

### Quick Reference

```bash
# Go unit tests
task tests-unit

# Python integration tests (no cloud resources)
task tests-integration

# Full deployment tests (requires AWS credentials, incurs costs)
task tests-deployment
```

## Code Quality

### Formatting

```bash
# Format all code (Go and Python)
task fmt
```

The project uses standard [Go](https://go.dev/) formatting (`go fmt`, `goimports`) and [Ruff](https://docs.astral.sh/ruff/) for Python.

### Linting

```bash
# Run all linters (Go, Python, Terraform)
task lint

# Auto-fix some Go linting issues
task lint-golang-fix
```

Configuration files:
- `.golangci.yml` - Go linting configuration
- `.tflint.hcl` - Terraform/OpenTofu linting
- `tests/pyproject.toml` - Python linting and type checking

### Best Practices

See [Best Practices](best_practices.md) for project-specific coding guidelines and conventions.

## Common Issues

**OpenTofu binary not found:**
- OpenTofu is resolved at runtime through the per-user runtime artifact cache.
- Use `exasol diag cache` to inspect cache state, `exasol cache clean --invalid` to remove artifacts that fail integrity checks, and `exasol cache clean --partial-downloads` to remove interrupted downloads.
- For direct tofu invocations in development workflows, use `task fmt-terraform` or `go run ./tools/tofu/main.go ...`.

**Windows fails with `tofu init -lockfile=readonly` due to missing hashes in `.terraform.lock.hcl`:**
- Regenerate the lockfiles with hashes for all supported platforms: run `task tofu-lock-update`.
- This updates the committed lockfile(s) under `assets/infrastructure/` without leaving temporary `.terraform/` directories behind.
- Presets that don't use OpenTofu (no `tofu:` section in `infrastructure.yaml` or no `.tf` files) are skipped.

**Tests fail with AWS errors:**
- Verify AWS credentials are configured (`AWS_PROFILE` or credential files)
- Check AWS permissions

## Dependency Management

```bash
# Update all dependencies (Go and Python)
task deps-update
```

Standard [Go module](https://go.dev/ref/mod) commands also work for managing Go dependencies directly.

## CI/CD and Releases

The project uses GitHub Actions for continuous integration and automated releases.

**CI Pipeline** - See [CI Documentation](ci.md) for details on:
- Automated linting and testing on every push
- Manual deployment tests

**Releases** - See [Release Process](release.md) for how to create and publish releases.
