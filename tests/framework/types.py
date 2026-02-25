# Copyright 2026 Exasol AG
# SPDX-License-Identifier: MIT

from typing import IO, TypedDict

StdIO = None | int | IO[str]


class SubprocessRunKwargs(TypedDict, total=False):
    input: str  # The input to pass over stdin.
    check: bool  # Raise an error on non-0 exit status.
    capture_output: bool  # Capture stdout and stder.
    stdin: StdIO
    stdout: StdIO
    stderr: StdIO
