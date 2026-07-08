# Copyright 2026 Exasol AG
# SPDX-License-Identifier: MIT

import pytest

_PROVIDER_MARKERS = {
    "provider_aws": "aws",
    "provider_azure": "azure",
    "provider_stackit": "stackit",
}


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


def pytest_collection_modifyitems(
    config: pytest.Config, items: list[pytest.Item]
) -> None:
    selected_infra = str(config.getoption("--infra"))
    selected_items: list[pytest.Item] = []
    deselected_items: list[pytest.Item] = []

    for item in items:
        provider_infras = {
            infra
            for marker_name, infra in _PROVIDER_MARKERS.items()
            if item.get_closest_marker(marker_name) is not None
        }
        if provider_infras and selected_infra not in provider_infras:
            deselected_items.append(item)
        else:
            selected_items.append(item)

    if deselected_items:
        config.hook.pytest_deselected(items=deselected_items)
        items[:] = selected_items
