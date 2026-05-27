# Copyright 2026 Exasol AG
# SPDX-License-Identifier: MIT

"""Shared fixtures for e2e tests.

The `e2e_deployment` fixture deploys a real cluster once per session and
shares it across read-only tests. Tests that mutate deployment state (stop,
destroy, interrupt) must own their own short-lived fixtures.
"""

import logging
from collections.abc import Iterator

import pytest

from framework.deployment import Deployment, StatusDatabaseReady
from framework.launcher import DeploymentConfig, Launcher


@pytest.fixture(scope="session")
def e2e_deployment(exasol_path: str, infra: str) -> Iterator[Deployment]:
    """Session-scoped real cluster shared by read-only e2e tests."""
    cluster_size = 2 if infra == "aws" else 1
    config = DeploymentConfig(infra=infra, cluster_size=cluster_size)
    deployment = Deployment(Launcher(exasol_path), config=config)
    try:
        deployment.deploy()
        if not deployment.has_status(StatusDatabaseReady):
            msg = f"Expected status `{StatusDatabaseReady}` after initial deploy"
            raise RuntimeError(msg)
        logging.info("e2e_deployment ready: id=%s", deployment.deployment_id())
        yield deployment
    finally:
        deployment.cleanup()
