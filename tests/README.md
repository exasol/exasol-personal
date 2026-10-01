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

### 2. Python Launcher Tests

- **Location:** `tests/launcher/`
- **Purpose:** Test launcher behavior through its supported interfaces without provisioned infrastructure
- **Run with:** `task tests-launcher` (from project root)
- **Speed:** Fast (seconds to minutes)
- **Resources:** No cloud resources provisioned
- **When to use:**
  - Verify CLI argument parsing and validation
  - Test command help output and error messages
  - Validate idempotency and state management
  - Check file creation and configuration handling

### 3. Python End-to-End Tests

Live tests live under `tests/live/` and are named for the launcher capability they
exercise. Recovery and smoke evidence remains beside that capability.

Tests request one of five execution boundaries: `deployment` for controlled state,
`local_deployment` for a real local runtime, `shared_live_deployment` for repeatable
read-only live behavior, `reusable_live_deployment` for vetted restorable mutations,
or `isolated_live_deployment` for destructive or non-restorable behavior. Pytest
automatically keeps shared and reusable deployments in exclusive groups and serializes
local deployment work when tests run concurrently.
The final pytest and GitHub summaries report how many deployments the run created.

Tests use `smoke`, `chaos`, `stress`, `providers`, and `platform` annotations only
when those facts affect selection. Product-facing evidence is linked with
`openspec("capability-id")`; this is a review aid, not strict one-to-one validation.
See the [testing architecture](../doc/testing-architecture-proposal.md) for the full
contract.

- **Run with:** `task tests-deployment`, or one of the focused tasks below.
- **Speed:** Slow (10-30+ minutes)
- **Resources:** **Creates real cloud resources (incurs costs!)**
- **When to use:**
  - Validate complete deployment workflows
  - Test actual cloud provisioning
  - Test local deployment behavior on supported hosts
  - Verify deployed database functionality
  - Pre-release validation

**Warning:** Cloud tests provision actual infrastructure and will incur charges. Use sparingly and clean up resources after testing.

## Running Tests

### Prerequisites

```bash
# First time setup - install Python dependencies
task tests-setup
```

This synchronizes the locked uv workspace environment needed for integration and deployment tests.
All integration and deployment tests are written using [pytest](https://pytest.org/).

### Running Tests

#### From Project Root (Recommended)

```bash
# Run all Go unit tests
task tests-unit

# Run cloud-free launcher tests
task tests-launcher

# Run broad live evidence on AWS, excluding opt-in stress tests
task tests-deployment

# Run the focused smoke path on AWS, Azure, or Exoscale
task tests-deployment-smoke INFRA=azure

# Run deliberately abnormal recovery scenarios
task tests-deployment-chaos INFRA=aws

# Run the local smoke path on a supported Linux or macOS host
task tests-deployment-local
```

#### From tests/ Directory

If you need more control over pytest execution:

```bash
cd tests

# Run all launcher tests
uv run --locked --no-build pytest --exasol-path=../bin/exasol tests/launcher

# Run specific test file
uv run --locked --no-build pytest --exasol-path=../bin/exasol tests/launcher/test_cli.py

# Run specific test
uv run --locked --no-build pytest --exasol-path=../bin/exasol tests/launcher/test_init.py::test_init_idempotent

# Run with verbose output
uv run --locked --no-build pytest --exasol-path=../bin/exasol tests/launcher -v

# Run broad cloud evidence with bounded deployment concurrency
export AWS_PROFILE=your-profile
EXASOL_RUN_CLOUD_DEPLOY_CASES=1 uv run --locked --no-build pytest -n 2 --dist loadgroup -rP -m "not stress" --exasol-path=../bin/exasol tests/live

# Run an Azure smoke path
EXASOL_RUN_CLOUD_DEPLOY_CASES=1 uv run --locked --no-build pytest -n 2 --dist loadgroup -rP -m smoke --infra=azure --exasol-path=../bin/exasol tests/live

# Run recovery evidence
EXASOL_RUN_CLOUD_DEPLOY_CASES=1 uv run --locked --no-build pytest -n 2 --dist loadgroup -rP -m chaos --infra=aws --exasol-path=../bin/exasol tests/live

# Run local evidence; pytest detects the host platform
uv run --locked --no-build pytest -n 2 --dist loadgroup -rP -m smoke --infra=local --exasol-path=../bin/exasol tests/live
```

Cloud tasks create billable resources. STACKIT live evidence is omitted until its
deployment path works correctly.

### Seeing the commands a test runs

Live logging is on at `INFO`, which does not include the individual launcher
invocations. Raise it to `DEBUG` and every command the suite executes is printed
before it starts, with its working directory when one is set:

```bash
uv run --locked --no-build pytest --exasol-path=../bin/exasol tests/launcher --log-cli-level=DEBUG
```

```
DEBUG executing command: ../bin/exasol presets list --json
DEBUG executing command (cwd: /tmp/.../work): /.../exasol status --json --deployment staging
```

This covers every command regardless of how a test invokes it — the shared
`run_command` helpers, the `framework` launcher, and tests that reach for
`subprocess` directly — because the root `conftest.py` logs at the single point
they all funnel through. Nothing needs to be passed to the helpers to opt in.

## Continuous Integration

GitHub Actions CI coverage:
- Unit tests and cloud-free launcher tests run on every push
- Deployment tests run manually via workflow dispatch; the local suite has Linux AMD64 and macOS ARM64 lanes
- See [CI Documentation](../doc/ci.md) for details
