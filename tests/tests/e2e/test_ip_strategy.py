import pytest

pytestmark = [pytest.mark.e2e]


@pytest.mark.skip(
    reason=(
        "Launcher does not expose an IP-strategy flag yet. "
        "Tracked in the test matrix as PDF #13; enable when the CLI gains a "
        "--public-ip / --restricted-ip option (or equivalent)."
    ),
)
def test_deploy_respects_ip_strategy() -> None:
    """PDF #13 (P3): placeholder; see module docstring."""
