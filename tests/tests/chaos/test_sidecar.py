# Copyright 2026 Exasol AG
# SPDX-License-Identifier: MIT

import pytest

from framework.sidecars import sidecar_deployment

pytestmark = pytest.mark.local_e2e


@pytest.mark.parametrize("interruption", ["before-create", "after-create"])
def test_sidecar_interrupted_enable_recovery(
    exasol_path: str,
    infra: str,
    interruption: str,
) -> None:
    # Given
    with sidecar_deployment(exasol_path, infra) as service:
        service.sidecar("enable")
        saved = (service.directory / "sidecars.yaml").read_bytes()
        if interruption == "before-create":
            service.podman("rm", "--force", service.container)
        else:
            service.podman("stop", service.container)
        # When
        service.deployment.start()
        # Then
        assert (service.directory / "sidecars.yaml").read_bytes() == saved
        assert service.sidecar("status")["hosts"][0]["running"]
        assert service.response().endswith("|initial-marker")
        assert "42" in service.sql()
