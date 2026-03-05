# Copyright 2026 Exasol AG
# SPDX-License-Identifier: MIT

import json
import logging
from pathlib import Path
from typing import Final

from pydantic import BaseModel

OUTPUTS_FILE: Final = "deployment.json"


class Database(BaseModel):
    dbPort: str  # noqa: N815
    uiPort: str  # noqa: N815
    url: str


class SSH(BaseModel):
    command: str
    keyName: str  # noqa: N815
    username: str


class Node(BaseModel):
    database: Database
    dnsName: str  # noqa: N815
    instanceId: str  # noqa: N815
    privateIp: str  # noqa: N815
    publicIp: str  # noqa: N815
    ssh: SSH


class Outputs(BaseModel):
    deploymentId: str  # noqa: N815
    nodes: dict[str, Node]


def _read_outputs(deployment_dir: str) -> str:
    deployment_dir_path = Path(deployment_dir)

    outputs_filepath = deployment_dir_path / OUTPUTS_FILE
    if not outputs_filepath.exists():
        msg = f"couldn't read the outputs file {OUTPUTS_FILE}"
        raise RuntimeError(msg)

    logging.info("Reading outputs file at: %s", outputs_filepath.name)
    with outputs_filepath.open() as outputs_file:
        return outputs_file.read()


def get_outputs(deployment_dir: str) -> Outputs:
    """Read and return the outputs file content."""
    logging.info("Getting outputs")

    outputs_raw = _read_outputs(deployment_dir)
    outputs_dict = json.loads(outputs_raw)

    return Outputs(**outputs_dict)
