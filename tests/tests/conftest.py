# Copyright 2026 Exasol AG
# SPDX-License-Identifier: MIT

import pytest


def pytest_addoption(parser: pytest.Parser) -> None:
    parser.addoption(
        "--exasol-path",
        type=str,
        required=False,
        action="store",
        default="exasol",
        help="Path to the exasol binary",
    )
    parser.addoption(
        "--infra",
        type=str,
        required=False,
        action="store",
        default="aws",
        choices=["aws", "azure", "exoscale", "stackit", "local"],
        help="Infrastructure preset to use for deployment tests",
    )
    parser.addoption(
        "--stackit-project-id",
        type=str,
        required=False,
        action="store",
        default=None,
        help="STACKIT project ID to put resources into",
    )


@pytest.fixture(scope="session")
def exasol_path(request: pytest.FixtureRequest) -> str:
    return str(request.config.getoption("--exasol-path"))


@pytest.fixture(scope="session")
def infra(request: pytest.FixtureRequest) -> str:
    return str(request.config.getoption("--infra"))


@pytest.fixture(scope="session")
def stackit_project_id(request: pytest.FixtureRequest) -> str | None:
    project_id = request.config.getoption("--stackit-project-id")
    if project_id is not None:
        return str(project_id)
    return None
