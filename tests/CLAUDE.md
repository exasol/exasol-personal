# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Scope

This directory holds the Python test suite for the Exasol Personal launcher (the Go CLI in `cmd/` / `bin/exasol`). It does **not** contain Go unit tests — those live next to the code they test as `*_test.go` and are run with `task tests-unit` from the repo root.

The unrelated `test/` (singular) directory at the repo root is gitignored runtime output from Terraform/deployment runs — not test sources.

## Layout

- `framework/` — Python bindings around the `exasol` binary. `Launcher` (`framework/launcher.py`) shells out to the CLI; `Deployment` (`framework/deployment.py`) wraps `Launcher` to manage a temporary deployment directory and real cloud resources, including retry logic for Azure NIC reservation errors and websocket-based DB readiness checks. Status string constants (e.g. `StatusDatabaseReady`, `StatusOperationInProgress`) live here and must match the CLI's `status --json` output.
- `tests/integration/` — CLI behavior tests. **No cloud resources provisioned.** Exercise arg parsing, idempotency, help output, file/config handling. Fast.
- `tests/e2e/` — End-to-end workflow tests. **Provisions real cloud infrastructure** (AWS/Azure/Exoscale). For workflow-focused end-to-end coverage (e.g. deploy idempotency, full user journeys) — sibling to `deployment/`, which targets infrastructure-specific contracts. Each test module typically owns its own session/module-scoped deployment fixture (see `test_deploy_idempotency.py`). Mark tests with `@pytest.mark.e2e`.
- `tests/deployment/` — End-to-end tests that **provision real cloud infrastructure** (AWS/Azure/Exoscale) and incur costs. Slow (10–30+ min). The `reusable_deployment` session fixture in `test_standard_deployment.py` deploys once and is shared across tests in that file; `test_custom.py` is for one-off deployments.
- `mock_version_server.py` — Standalone HTTP server used by `test_version_check.py` via the `mock_version_server` fixture in `tests/integration/conftest.py`. Started as a subprocess on port 18080 and configured via POST to `/set-package-data`.

## Running tests

Prefer `task` from the repo root; it handles `tests-setup` (poetry sync), binary path, and infra flags. Drop down to `poetry run pytest` from `tests/` only when you need finer control (e.g. running a single test).

```bash
# Repo-root entry points
task tests-setup                               # poetry sync, first-time only
task tests-integration
task tests-e2e INFRA=aws                       # also: TEST=test_foo.py::test_bar
task tests-deployment INFRA=aws                # also: azure, exoscale
task tests-deployment INFRA=aws TEST=test_standard_deployment.py::test_license_session_limit
task tests-deployment-infrastructure INFRA=aws # -m infrastructure_e2e
task tests-deployment-installation INFRA=aws   # -m installation_e2e

# From tests/ directly
poetry run pytest --exasol-path=../bin/exasol tests/integration
poetry run pytest --exasol-path=../bin/exasol --infra=azure tests/deployment
poetry run pytest --exasol-path=../bin/exasol --infra=aws -m "infrastructure_e2e" tests/deployment
```

`--exasol-path` and `--infra` are custom pytest options registered in `tests/conftest.py` and exposed as session fixtures of the same names. The `bin/exasol` binary must be built first (`task build` from the root).

## Pytest markers

Declared in `pyproject.toml`; do not invent new ones without adding them there or `pytest` will warn.

- `launcher_tests` — CLI/behavioral contracts without cloud deployment
- `e2e` — workflow-level end-to-end tests in `tests/e2e/` (cloud)
- `installation_e2e` — installation-level end-to-end (cloud)
- `infrastructure_e2e` — cloud infrastructure end-to-end
- `provider_aws`, `provider_azure` — provider-specific assertions

The `INFRA` env/flag selects which preset to deploy against; provider-specific tests should additionally gate on `provider_aws` / `provider_azure` markers and `pytest.mark.skipif` against the `infra` fixture when needed.

## Conventions

- **Given / When / Then** comments in test bodies (`# Given`, `# When`, `# Then`) — this is enforced by convention across the repo, see `tests/README.md`.
- Strict typing: `mypy --strict` (Python 3.13) and `ruff` with `select = ["ALL"]`. Run `task lint-mypy` and `task lint-ruff` before declaring work done; `task lint-ruff-fix` autofixes most ruff complaints. Note that `D100`–`D104`, `D107`, `S101`, `S603`, `LOG015` are intentionally ignored — don't add docstrings just to satisfy lint.
- Subprocess kwargs flow through `framework.types.SubprocessRunKwargs` (a `TypedDict`) so `Launcher.run_command(..., **kwargs)` stays typed under strict mypy. Match this pattern when adding new launcher methods.
- License header `# Copyright 2026 Exasol AG / # SPDX-License-Identifier: MIT` is required on every `.py` file and enforced by `task lint-licenses`.

## Deployment-test gotchas

- Deployment tests create real resources. The `Deployment` class auto-cleans via `destroy` with retry, but a crashed test runner may leave resources behind — verify in the cloud console after failures.
- Cluster size in `test_standard_deployment.py::reusable_deployment` is 2 for AWS and 1 elsewhere; preserve this if editing.
- Azure deployments need a location: passed via `DeploymentConfig.location`, or env vars `TF_VAR_LOCATION` / `AZURE_LOCATION`. AWS uses `AWS_PROFILE`.
- The session-scoped `reusable_deployment` fixture means tests in that module **share state**. Tests that mutate the deployment (stop/start/destroy) must restore it or be ordered last.
