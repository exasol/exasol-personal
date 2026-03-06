#!/usr/bin/env python3
# Copyright 2026 Exasol AG
# SPDX-License-Identifier: MIT
"""Mock version server for testing version check functionality."""

import argparse
import json
import logging
import os
import sys
import threading
from http.server import BaseHTTPRequestHandler, HTTPServer

# Global storage for response data (with thread lock)
stored_response: bytes | None = None
data_lock = threading.RLock()
version_check_count = 0

logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s %(levelname)s %(message)s",
    datefmt="%b %d %H:%M:%S",
)
logger = logging.getLogger(__name__)


class VersionServerHandler(BaseHTTPRequestHandler):
    """HTTP request handler for mock version server."""

    def log_message(self, fmt: str, *args: object) -> None:
        """Override to use our logger instead of stderr."""
        logger.info(fmt, *args)

    def do_GET(self) -> None:
        """Handle GET requests to /version-check endpoint."""
        if self.path.startswith("/version-check-count"):
            self._handle_version_check_count()
        elif self.path.startswith("/version-check"):
            self._handle_version_check()
        else:
            self.send_error(404, "Not Found")

    def do_POST(self) -> None:
        """Handle POST requests to /set-package-data endpoint."""
        if self.path == "/set-package-data":
            self._handle_set_package_data()
        else:
            self.send_error(404, "Not Found")

    def _handle_version_check_count(self) -> None:
        """Handle version check GET requests."""
        global version_check_count  # noqa: PLW0603

        count = 0

        with data_lock:
            count = version_check_count
            version_check_count = 0

        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.end_headers()
        self.wfile.write(f'{{"count": {count}}}'.encode())

    def _handle_version_check(self) -> None:
        """Handle version check GET requests."""
        global version_check_count  # noqa: PLW0603

        # Parse query parameters
        from urllib.parse import parse_qs, urlparse  # noqa: PLC0415

        parsed = urlparse(self.path)
        params = parse_qs(parsed.query)

        category = params.get("category", [""])[0]
        operating_system = params.get("operatingSystem", [""])[0]
        architecture = params.get("architecture", [""])[0]
        version = params.get("version", [""])[0]
        identity = params.get("identity", [""])[0]

        logger.info(
            "received version check request: category=%s operatingSystem=%s "
            "architecture=%s version=%s identity=%s",
            category,
            operating_system,
            architecture,
            version,
            identity,
        )

        with data_lock:
            version_check_count += 1
            response = stored_response
            has_data = response is not None
            response_bytes = len(response) if response else 0

        logger.info(
            "checking stored response: hasData=%s bytes=%s", has_data, response_bytes
        )

        if response is None:
            self.send_response(404)
            self.send_header("Content-Type", "application/json")
            self.end_headers()
            self.wfile.write(b'{"error":"No data available"}')
            return

        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.end_headers()
        self.wfile.write(response)

    def _handle_set_package_data(self) -> None:
        """Handle POST requests to set package data."""
        global stored_response  # noqa: PLW0603

        content_length = int(self.headers.get("Content-Length", 0))
        body = self.rfile.read(content_length)

        # Validate it's valid JSON
        try:
            json.loads(body)
        except json.JSONDecodeError:
            self.send_error(400, "Invalid JSON")
            return

        with data_lock:
            stored_response = body

        logger.info(
            "stored response data: bytes=%s data=%s", len(body), body.decode("utf-8")
        )

        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.end_headers()
        response = json.dumps(
            {"status": "success", "message": "Data stored successfully"}
        )
        self.wfile.write(response.encode("utf-8"))


def main() -> int:
    """Run the mock version server."""
    parser = argparse.ArgumentParser(description="Mock version server for testing")
    parser.add_argument(
        "-port",
        "--port",
        type=int,
        default=8080,
        help="Port to listen on (default: 8080)",
    )
    args = parser.parse_args()

    port = args.port
    if "PORT" in os.environ:
        port = int(os.environ["PORT"])

    server_address = ("", port)
    httpd = HTTPServer(server_address, VersionServerHandler)

    logger.info("Mock version server starting on :%s", port)

    try:
        httpd.serve_forever()
    except KeyboardInterrupt:
        logger.info("Shutting down server")
        httpd.shutdown()
        return 0
    except Exception:
        logger.exception("Server error")
        return 1

    return 0


if __name__ == "__main__":
    sys.exit(main())
