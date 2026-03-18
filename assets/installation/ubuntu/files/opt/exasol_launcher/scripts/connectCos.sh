#!/usr/bin/env bash
set -Eeuo pipefail

SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)

# shellcheck source=./shared_post_install.sh
source "${SCRIPT_DIR}/shared_post_install.sh"

PLAY_ID="$("$C4_PATH" config -e .play.id)"

exec "$C4_PATH" connect -s cos -i "$PLAY_ID"
