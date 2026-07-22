# Copyright 2026 Exasol AG
# SPDX-License-Identifier: MIT

"""STACKIT provider deployments: single-node, sizing, object storage, e2e suite."""

import pytest

from tests.testcase_helpers import skip_unless_infra


@pytest.mark.provider_stackit
@pytest.mark.infrastructure_e2e
@pytest.mark.installation_e2e
def test_default_single_node_deployment(
    infra: str, stackit_project_id: str | None
) -> None:
    skip_unless_infra(infra, "stackit")
    if not stackit_project_id:
        pytest.skip("requires --stackit-project-id and STACKIT credentials")
    # A full STACKIT provisioning run is a live cloud test; it is covered by the
    # deployment suite (see TC-STACKIT-04).
    pytest.skip("requires live STACKIT provisioning (see tests/deployment)")


@pytest.mark.provider_stackit
@pytest.mark.infrastructure_e2e
def test_custom_region_and_sizing(infra: str, stackit_project_id: str | None) -> None:
    skip_unless_infra(infra, "stackit")
    if not stackit_project_id:
        pytest.skip("requires --stackit-project-id and STACKIT credentials")

    pytest.skip("requires live STACKIT provisioning of a multi-node cluster")


@pytest.mark.provider_stackit
@pytest.mark.infrastructure_e2e
def test_bootstrap_via_object_storage(
    infra: str, stackit_project_id: str | None
) -> None:
    skip_unless_infra(infra, "stackit")
    if not stackit_project_id:
        pytest.skip("requires --stackit-project-id and STACKIT credentials")

    pytest.skip("requires live STACKIT provisioning and cloud-init log inspection")


@pytest.mark.provider_stackit
@pytest.mark.infrastructure_e2e
def test_stackit_deployment_suite(infra: str, stackit_project_id: str | None) -> None:
    skip_unless_infra(infra, "stackit")
    if not stackit_project_id:
        pytest.skip("requires --stackit-project-id and STACKIT credentials")

    # The behaviour is exercised by tests/deployment when run against STACKIT;
    # this placeholder documents the entry point rather than re-running it.
    pytest.skip("run the tests/deployment suite with --infra=stackit")
