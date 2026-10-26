#!/usr/bin/env bash
# Remote wrapper invoked by the launcher to start monitoring on the node.

set -euo pipefail

if [[ ! -x /opt/exasol_launcher/scripts/monitorInstallation.sh ]]; then
  echo "monitoring script not found" >&2
  exit 1
fi

if [[ ! -f /var/lib/exasol_launcher/state/post_install.complete ]] &&
   systemctl is-failed --quiet exasol_launcher_post_install.service; then
  echo "EXASOL-INSTALL-STEP: Retrying unfinished post-installation"
  sudo systemctl reset-failed exasol_launcher_post_install.service exasol_launcher.target
  sudo systemctl start --no-block exasol_launcher_post_install.service exasol_launcher.target
fi

exec /opt/exasol_launcher/scripts/monitorInstallation.sh
