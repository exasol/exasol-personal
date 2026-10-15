# Copyright 2026 Exasol AG
# SPDX-License-Identifier: MIT

import json
import platform
import shutil
import subprocess
import sys
from pathlib import Path
from subprocess import CalledProcessError
from typing import Final

import pytest

from tests.testcase_helpers import windows_podman_path

from .helpers import (
    compatible_preset_pair_or_skip,
    export_preset,
    first_infrastructure_preset_id_or_skip,
    preset_description_or_skip,
    run_command,
)

LOCAL_MINIMUM_MEMORY_MB = 4096
OLD_FIXED_DEFAULT_MB = 2048
LOCAL_MACHINE = platform.machine().lower()
IS_MACOS_APPLE_SILICON = sys.platform == "darwin" and LOCAL_MACHINE in {
    "arm64",
    "aarch64",
}
IS_LINUX_LOCAL_PLATFORM = sys.platform.startswith("linux") and LOCAL_MACHINE in {
    "amd64",
    "x86_64",
    "arm64",
    "aarch64",
}
IS_WINDOWS_LOCAL_PLATFORM = sys.platform.startswith("win") and LOCAL_MACHINE in {
    "amd64",
    "x86_64",
}
# Linux and Windows both run the database directly through host Podman, so
# they share the "no VM sizing" configuration surface.
IS_HOST_LOCAL_PLATFORM = IS_LINUX_LOCAL_PLATFORM or IS_WINDOWS_LOCAL_PLATFORM
IS_SUPPORTED_LOCAL_PLATFORM = IS_MACOS_APPLE_SILICON or IS_HOST_LOCAL_PLATFORM
LOCAL_VM_SIZING_FLAGS = {"--cpu-count", "--memory-mb", "--data-size-gb"}


def test_install_requires_infra_preset_arg(exasol_path: str) -> None:
    # Given the install command

    # When it is invoked without arguments
    with pytest.raises(CalledProcessError) as exc:
        run_command([exasol_path, "install"])

    # Then it fails because the required infra preset argument is missing
    assert exc.value.returncode != 0
    assert (
        "requires" in (exc.value.stderr or "").lower()
        or "accepts" in (exc.value.stderr or "").lower()
    )


def test_install_help(exasol_path: str) -> None:
    # Given the install command

    # When help is invoked
    result = run_command([exasol_path, "install", "--help"])
    output: str = result.stdout.strip()

    # Then the output explains the command
    assert "Initialize, apply configuration, and deploy Exasol in one step" in output

    # Then I see which preset names I can pass
    infra_id = first_infrastructure_preset_id_or_skip(exasol_path)
    assert "Available infrastructure presets:" in output
    assert infra_id in output
    assert "Available installation presets:" in output
    assert "exasol presets" in output

    # Then I see how the presets can be combined
    assert "Compatibility matrix" in output


def test_install_help_describes_the_selected_infrastructure_preset(
    exasol_path: str,
) -> None:
    # Given an infrastructure preset
    infra_id = first_infrastructure_preset_id_or_skip(exasol_path)
    description = preset_description_or_skip(exasol_path, "infrastructures", infra_id)

    # When help is invoked for that preset
    result = run_command([exasol_path, "install", infra_id, "--help"])
    output: str = result.stdout.strip()

    # Then the help describes the selected preset and how to combine it
    assert f"Infrastructure preset `{infra_id}`:" in output
    assert description in output
    assert "Compatible installation presets:" in output

    # Then the help exemplifies the command with the selected preset
    assert f"exasol install {infra_id}" in output

    # Then the help does not repeat the overview of every preset
    assert "Available infrastructure presets:" not in output
    assert "Compatibility matrix" not in output

    # Then preset discovery stays reachable
    assert "exasol presets" in output


def test_install_help_describes_both_selected_presets(exasol_path: str) -> None:
    # Given a compatible infrastructure and installation preset pair
    infra_id, install_id = compatible_preset_pair_or_skip(exasol_path)
    install_description = preset_description_or_skip(
        exasol_path, "installations", install_id
    )

    # When help is invoked for both presets
    result = run_command([exasol_path, "install", infra_id, install_id, "--help"])
    output: str = result.stdout.strip()

    # Then the help describes both selected presets
    assert f"Infrastructure preset `{infra_id}`:" in output
    assert f"Installation preset `{install_id}`:" in output
    assert install_description in output

    # Then the help exemplifies the selected pair
    assert f"exasol install {infra_id} {install_id}" in output

    # Then the help does not repeat the overview of every preset
    assert "Available installation presets:" not in output
    assert "Compatibility matrix" not in output


def test_install_help_describes_a_preset_selected_by_path(
    exasol_path: str, tmp_path: Path
) -> None:
    # Given an infrastructure preset exported to a directory
    infra_id = first_infrastructure_preset_id_or_skip(exasol_path)
    description = preset_description_or_skip(exasol_path, "infrastructures", infra_id)
    infra_dir = tmp_path / "infra_export"
    infra_dir.mkdir()
    export_preset(exasol_path, infra_id, "infrastructure", str(infra_dir))

    # When help is invoked with that directory as the preset argument
    result = run_command([exasol_path, "install", str(infra_dir), "--help"])
    output: str = result.stdout.strip()

    # Then the help identifies the preset by its path and describes it
    assert f"Infrastructure preset `{infra_dir}`:" in output
    assert description in output
    assert "Compatible installation presets:" in output

    # Then the help does not repeat the overview of every preset
    assert "Available infrastructure presets:" not in output


def test_install_help_quotes_a_preset_path_containing_spaces(
    exasol_path: str, tmp_path: Path
) -> None:
    # Given an infrastructure preset exported to a directory whose path contains a space
    infra_id = first_infrastructure_preset_id_or_skip(exasol_path)
    infra_dir = tmp_path / "my preset"
    infra_dir.mkdir()
    export_preset(exasol_path, infra_id, "infrastructure", str(infra_dir))

    # When help is invoked with that directory as the preset argument
    result = run_command([exasol_path, "install", str(infra_dir), "--help"])
    output: str = result.stdout.strip()

    # Then the examples keep the path as a single argument
    assert f'exasol install "{infra_dir}"' in output

    # Then no example splits the path across two arguments
    assert f"exasol install {infra_dir}\n" not in output


def test_install_help_keeps_the_overview_for_an_unresolvable_installation_preset(
    exasol_path: str,
) -> None:
    # Given a resolvable infrastructure preset, and an installation preset
    # argument that names nothing
    infra_id = first_infrastructure_preset_id_or_skip(exasol_path)

    # When help is invoked for both
    result = run_command([exasol_path, "install", infra_id, "no-such-preset", "--help"])
    output: str = result.stdout.strip()

    # Then the overview of every preset is shown, not a description of one of them
    assert "Available infrastructure presets:" in output
    assert "Compatibility matrix" in output

    # Then no example advertises the preset that could not be described
    assert "no-such-preset" not in output


def test_install_help_keeps_the_overview_for_an_unresolvable_preset(
    exasol_path: str,
) -> None:
    # Given a preset argument naming neither an embedded preset nor a preset directory

    # When help is invoked for it
    result = run_command([exasol_path, "install", "no-such-preset", "--help"])
    output: str = result.stdout.strip()

    # Then help is printed with the overview of every preset
    assert "Available infrastructure presets:" in output
    assert "Available installation presets:" in output
    assert "Compatibility matrix" in output


def test_install_executes_init_step(exasol_path: str, tmp_path: Path) -> None:
    # Given a non-empty deployment directory
    deployment_dir = tmp_path / "deployment"
    deployment_dir.mkdir()
    (deployment_dir / "somefile.txt").write_text("x")

    # Given an infrastructure preset ID
    infra_id = first_infrastructure_preset_id_or_skip(exasol_path)

    # When the install command is invoked
    args = [
        exasol_path,
        "install",
        infra_id,
        "--deployment-dir",
        str(deployment_dir),
    ]
    with pytest.raises(CalledProcessError) as excinfo:
        run_command(args)

    # Then it fails during initialization (proving init ran)
    assert excinfo.value.returncode != 0
    stderr = (excinfo.value.stderr or "").lower()
    assert "initialization failed" in stderr
    assert "deployment directory is not empty" in stderr


@pytest.mark.skipif(
    IS_SUPPORTED_LOCAL_PLATFORM,
    reason="local deployments are supported on this platform",
)
def test_init_local_rejects_unsupported_platform_before_writing_files(
    exasol_path: str, tmp_path: Path
) -> None:
    # Given an empty deployment directory on an unsupported local platform
    deployment_dir = tmp_path / "deployment"
    deployment_dir.mkdir()

    # When init is invoked for the local preset
    args = [
        exasol_path,
        "init",
        "local",
        "--deployment-dir",
        str(deployment_dir),
        "--no-launcher-version-check",
    ]
    with pytest.raises(CalledProcessError) as exc:
        run_command(args)

    # Then it fails before writing deployment state
    stderr = exc.value.stderr.lower()
    assert (
        "local deployments are only supported on macos apple silicon, "
        "linux amd64/arm64, and windows amd64" in stderr
    )
    assert list(deployment_dir.iterdir()) == []


@pytest.mark.skipif(
    not IS_HOST_LOCAL_PLATFORM,
    reason="host configuration is only exposed on supported host platforms",
)
def test_init_local_host_help_exposes_only_host_parameters(exasol_path: str) -> None:
    # Given the local preset on a supported direct-host platform

    # When init help is requested
    result = run_command([exasol_path, "init", "local", "--help"])

    # Then the host port option is exposed without VM sizing options
    assert "--ports" in result.stdout
    for flag in LOCAL_VM_SIZING_FLAGS:
        assert flag not in result.stdout


@pytest.mark.skipif(
    not IS_HOST_LOCAL_PLATFORM,
    reason="host configuration is only exposed on supported host platforms",
)
def test_init_local_host_configuration_omits_vm_sizing(
    exasol_path: str, tmp_path: Path
) -> None:
    # Given an initialized local deployment on a direct-host platform
    deployment_dir = tmp_path / "deployment"
    run_command(
        [
            exasol_path,
            "init",
            "local",
            "--deployment-dir",
            str(deployment_dir),
            "--no-launcher-version-check",
        ]
    )

    # When the active configuration is queried
    result = run_command(
        [
            exasol_path,
            "config",
            "get",
            "--json",
            "--deployment-dir",
            str(deployment_dir),
        ]
    )

    # Then only the host-relevant local option is reported
    options = json.loads(result.stdout)["infrastructure"]["options"]
    assert set(options) == {"ports"}
    service, separator, raw_port = options["ports"].partition(":")
    assert service == "db"
    assert separator == ":"
    assert int(raw_port) > 0


@pytest.mark.openspec("exasol-local-deployment")
@pytest.mark.skipif(
    not IS_HOST_LOCAL_PLATFORM,
    reason="shell access is refused only by the direct-host local runtime",
)
@pytest.mark.parametrize(
    ("shell", "kind"), [("host", "host shells"), ("container", "container shells")]
)
def test_local_shell_is_refused_on_direct_host_platforms(
    exasol_path: str, tmp_path: Path, shell: str, kind: str
) -> None:
    # Given an initialized local deployment on a direct-host platform
    deployment_dir = tmp_path / "deployment"
    run_command(
        [
            exasol_path,
            "init",
            "local",
            "--deployment-dir",
            str(deployment_dir),
            "--no-launcher-version-check",
        ]
    )

    # When a shell is requested
    result = subprocess.run(
        [exasol_path, "shell", shell, "--deployment-dir", str(deployment_dir)],
        capture_output=True,
        text=True,
        encoding="utf-8",
        stdin=subprocess.DEVNULL,
        check=False,
    )

    # Then it fails, naming the platform and stating the shell is unsupported
    assert result.returncode != 0
    assert platform.system().lower() in result.stderr
    assert f"does not support {kind}" in result.stderr


@pytest.mark.skipif(
    not IS_MACOS_APPLE_SILICON,
    reason="local memory sizing applies only to the macOS VM runtime",
)
def test_init_local_accepts_explicit_minimum_memory(
    exasol_path: str, tmp_path: Path
) -> None:
    # Given a local macOS VM deployment directory
    deployment_dir = tmp_path / "deployment"
    deployment_dir.mkdir()

    # When init is invoked with the minimum supported memory
    result = run_command(
        [
            exasol_path,
            "init",
            "local",
            "--deployment-dir",
            str(deployment_dir),
            "--memory-mb",
            "4096",
        ]
    )

    # Then the deployment is initialized with that value
    assert result.returncode == 0
    config_result = run_command(
        [
            exasol_path,
            "config",
            "get",
            "--json",
            "memory-mb",
            "--deployment-dir",
            str(deployment_dir),
        ]
    )
    config_data = json.loads(config_result.stdout)
    configured_memory_mb = config_data["infrastructure"]["options"]["memory-mb"]
    assert configured_memory_mb == LOCAL_MINIMUM_MEMORY_MB


@pytest.mark.skipif(
    not IS_MACOS_APPLE_SILICON,
    reason="local memory sizing applies only to the macOS VM runtime",
)
def test_init_local_defaults_to_half_host_memory(
    exasol_path: str, tmp_path: Path
) -> None:
    deployment_dir = tmp_path / "deployment"
    run_command(
        [
            exasol_path,
            "init",
            "local",
            "--deployment-dir",
            str(deployment_dir),
            "--no-launcher-version-check",
        ]
    )

    config_result = run_command(
        [
            exasol_path,
            "config",
            "get",
            "--json",
            "memory-mb",
            "--deployment-dir",
            str(deployment_dir),
        ]
    )
    memory_mb = json.loads(config_result.stdout)["infrastructure"]["options"][
        "memory-mb"
    ]

    assert memory_mb != OLD_FIXED_DEFAULT_MB
    assert memory_mb >= LOCAL_MINIMUM_MEMORY_MB


@pytest.mark.skipif(
    not IS_MACOS_APPLE_SILICON,
    reason="local memory sizing applies only to the macOS VM runtime",
)
def test_init_local_rejects_memory_below_minimum(
    exasol_path: str, tmp_path: Path
) -> None:
    # Given a local macOS VM deployment directory
    deployment_dir = tmp_path / "deployment"
    deployment_dir.mkdir()

    # When init is invoked below the supported minimum memory
    args = [
        exasol_path,
        "init",
        "local",
        "--deployment-dir",
        str(deployment_dir),
        "--memory-mb",
        "4095",
    ]
    with pytest.raises(CalledProcessError) as exc:
        run_command(args)

    # Then the user sees the minimum-memory validation message
    assert (
        "local memory-mb must be at least 4096 mb" in (exc.value.stderr or "").lower()
    )
    # Then validation happened before extraction, leaving the directory empty
    # so a corrected retry is not blocked by leftover preset files.
    assert list(deployment_dir.iterdir()) == []


# The flags the Windows lifecycle test drives, kept next to it so the two stay
# in step. Appending --help makes the launcher parse flags and exit without
# side effects, so an unknown flag surfaces as a non-zero exit anywhere.
WINDOWS_LIFECYCLE_FLAG_PROBES: Final = [
    (
        "install",
        ["local", "--auto-approve", "--no-launcher-version-check", "--verbose"],
    ),
    ("connect", ["--csv", "--command", "SELECT 1;"]),
    ("stop", ["--json"]),
    ("start", ["--json", "--auto-approve"]),
    ("destroy", ["--auto-approve", "--verbose"]),
]


@pytest.mark.parametrize(("command", "args"), WINDOWS_LIFECYCLE_FLAG_PROBES)
def test_windows_lifecycle_flags_are_accepted(
    exasol_path: str, command: str, args: list[str]
) -> None:
    """Every flag the Windows lifecycle test uses must exist.

    That test is skipped everywhere but Windows, so a flag that does not exist
    is invisible until CI runs it. This probe runs on all platforms.
    """
    # When the flags are parsed and the command exits at --help
    result = run_command([exasol_path, command, *args, "--help"])

    # Then parsing succeeded
    assert result.returncode == 0


@pytest.mark.openspec("windows-host-runtime-environment")
@pytest.mark.skipif(
    not IS_WINDOWS_LOCAL_PLATFORM,
    reason="Podman host preparation is approval-gated only on Windows",
)
def test_install_local_without_terminal_refuses_host_preparation(
    exasol_path: str, tmp_path: Path
) -> None:
    """A run with no terminal and no --auto-approve must not install Podman."""
    # Given a Windows host where Podman is absent and winget could install it
    if shutil.which("podman") is not None or windows_podman_path().is_file():
        pytest.skip("Podman is already installed, so no host preparation is needed")
    if shutil.which("winget") is None:
        pytest.skip("winget is unavailable, so the approval step is never reached")
    deployment_dir = tmp_path / "deployment"

    # When install runs without a terminal and without --auto-approve
    result = subprocess.run(
        [
            exasol_path,
            "install",
            "local",
            "--deployment-dir",
            str(deployment_dir),
            "--no-launcher-version-check",
        ],
        capture_output=True,
        text=True,
        encoding="utf-8",
        stdin=subprocess.DEVNULL,
        check=False,
    )

    # Then it fails, explaining that approval is required and how to give it
    assert result.returncode != 0
    assert "requires approval" in result.stderr
    assert "--auto-approve" in result.stderr

    # And Podman was not installed
    assert not windows_podman_path().is_file()
