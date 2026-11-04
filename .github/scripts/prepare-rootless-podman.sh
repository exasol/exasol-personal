#!/usr/bin/env bash
set -euo pipefail

source /etc/os-release
profile=/etc/apparmor.d/usr.bin.pasta
runtime_info=$(podman info --format json)
printf '%s\n' "$runtime_info"
affected=$(python3 -c '
import json, sys
info = json.load(sys.stdin)
host = info["host"]
print(str(host["security"]["rootless"]
          and host.get("rootlessNetworkCmd") == "pasta"
          and info["version"]["Version"].startswith("5.")).lower())
' <<< "$runtime_info")

# https://github.com/actions/runner-images/issues/15182
if [[ "$ID" == ubuntu && "$VERSION_ID" == 26.04 && "$affected" == true ]] &&
  [[ "$(cat /sys/module/apparmor/parameters/enabled)" == Y ]]; then
  if [[ ! -f "$profile" ]] || ! grep -Eq '^profile pasta /usr/bin/pasta' "$profile"; then
    echo '::error::Unexpected pasta AppArmor profile; cannot apply the Ubuntu 26.04 correction'
    exit 1
  fi
  if ! grep -Eq '^[[:space:]]*signal[[:space:]]+\(receive\)[[:space:]]+set=\(term\)[[:space:]]+peer=podman,' "$profile"; then
    sudo sed -i '$ i\  signal (receive) set=(term) peer=podman,' "$profile"
  fi
  sudo apparmor_parser -r "$profile"
fi

# This exercises creation and teardown without leaving a container or network.
podman unshare --rootless-netns true
