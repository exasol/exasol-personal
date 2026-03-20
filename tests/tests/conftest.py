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
        choices=["aws", "azure", "exoscale"],
        help="Infrastructure preset to use for deployment tests",
    )


@pytest.fixture(scope="session")
def exasol_path(request: pytest.FixtureRequest) -> str:
    return str(request.config.getoption("--exasol-path"))


@pytest.fixture(scope="session")
def infra(request: pytest.FixtureRequest) -> str:
    return str(request.config.getoption("--infra"))
