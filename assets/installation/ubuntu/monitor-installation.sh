#!/usr/bin/env bash
# Remote wrapper invoked by the launcher to start monitoring on the node.

set -euo pipefail

if [[ ! -x /opt/exasol_launcher/scripts/monitorInstallation.sh ]]; then
  echo "monitoring script not found" >&2
  exit 1
fi

exec /opt/exasol_launcher/scripts/monitorInstallation.sh
