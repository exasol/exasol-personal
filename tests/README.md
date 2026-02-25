# Testing Guide

This directory contains integration and deployment tests for the Exasol Personal launcher.

## Testing Principles

When adding or changing tests or test infrastructure, follow these principles:

**Match Existing Patterns** - Inspect the codebase before adding new tests. Follow established file organization, naming conventions, and test structure patterns - or if they don't exist, establish these patterns.

**Test Behavioral Space** - Cover the full range of behaviors: success cases, failure cases, edge cases, and repeated operations. One test per acceptance criterion is often insufficient.

**Test Contracts, Not Artifacts** - Focus on stable contracts and guarantees (what MUST always be true), not incidental implementation details (what happens to be true now).

**Separate Concerns** - Organize tests by the component or command they test. Related functionality belongs together; unrelated functionality should be separated.

**Use Clear Names** - Test names and docstrings should explain what behavior is being tested.

**Keep Tests Independent** - Each test should be runnable in isolation without depending on other tests.

### Given / When / Then Structure

Use the "Given / When / Then" pattern to make test intent explicit and consistent across the repo. Express these phases with short comments inside the test body:

- Given: setup and preconditions (fixtures, stubs, initial state)
- When: the action under test (a function call, a command execution, or a signal emitted)
- Then: assertions about observable behavior and outcomes (results, side effects, logs)

Guidelines:
- Keep the phases small and focused; avoid mixing setup and assertions.
- Prefer clear comments over lengthy names when the flow needs emphasis.
- For Go tests, add inline `// Given`, `// When`, `// Then` comments.
- For Python pytest tests, use `# Given`, `# When`, `# Then` comments.
- If a test has multiple actions and assertions, repeat `When/Then` pairs as needed.

## Test Strategy

The project uses multiple layers of testing to ensure quality at different levels:

### 1. Go Unit Tests

- **Location:** `*_test.go` files throughout the codebase
- **Purpose:** Test individual Go functions and packages in isolation
- **Run with:** `task tests-unit` (from project root)
- **Speed:** Fast (seconds)
- **Resources:** No external dependencies
- **When to use:** Test core logic, data structures, utility functions

### 2. Python Integration Tests

- **Location:** `tests/integration/`
- **Purpose:** Test CLI behavior and command interactions without cloud resources
- **Run with:** `task tests-integration` (from project root)
- **Speed:** Fast (seconds to minutes)
- **Resources:** No cloud resources provisioned
- **When to use:**
  - Verify CLI argument parsing and validation
  - Test command help output and error messages
  - Validate idempotency and state management
  - Check file creation and configuration handling
  
### 3. Python Deployment Tests

- **Location:** `tests/deployment/`
- **Purpose:** Full end-to-end tests with real AWS infrastructure
- **Run with:** `task tests-deployment` (from project root)
- **Speed:** Slow (10-30+ minutes)
- **Resources:** **Creates real AWS resources (incurs costs!)**
- **When to use:**
  - Validate complete deployment workflows
  - Test actual cloud provisioning
  - Verify deployed database functionality
  - Pre-release validation

**Warning:** Deployment tests provision actual AWS infrastructure and will incur charges. Use sparingly and clean up resources after testing.

## Running Tests

### Prerequisites

```bash
# First time setup - install Python dependencies
task tests-setup
```

This installs Poetry and Python dependencies needed for integration and deployment tests.
All integration and deployment tests are written using [pytest](https://pytest.org/).

### Running Tests

#### From Project Root (Recommended)

```bash
# Run all Go unit tests
task tests-unit

# Run Python integration tests
task tests-integration

# Run Python deployment tests (requires AWS credentials)
task tests-deployment
```

#### From tests/ Directory

If you need more control over pytest execution:

```bash
cd tests

# Run all integration tests
poetry run pytest --exasol-path=../bin/exasol tests/integration

# Run specific test file
poetry run pytest --exasol-path=../bin/exasol tests/integration/test_cli.py

# Run specific test
poetry run pytest --exasol-path=../bin/exasol tests/integration/test_cli.py::test_init_idempotent

# Run with verbose output
poetry run pytest --exasol-path=../bin/exasol tests/integration -v

# Run deployment tests (requires AWS credentials)
export AWS_PROFILE=your-profile
poetry run pytest --exasol-path=../bin/exasol tests/deployment
```

## Continuous Integration

All tests run automatically in GitHub Actions CI:
- Unit tests and integration tests run on every push
- Deployment tests run manually via workflow dispatch
- See [CI Documentation](../doc/ci.md) for details
