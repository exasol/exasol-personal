#!/usr/bin/env bash
set -Eeuo pipefail
# This script installs the TLS certificate using confd_client
# Note that this is only run on one node (n11)

SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=./logging.sh
source "${SCRIPT_DIR}/logging.sh"

# shellcheck source=./config.sh
source "${SCRIPT_DIR}/config.sh"

log_step_info "Configuring TLS certificates..."

# shared_post_install.sh provides C4_PATH and PLAY_ID
log_substep_info "Looking up cluster PLAY_ID"
source "${SCRIPT_DIR}/shared_post_install.sh"

log_substep_info "Uploading TLS certificate"
# Doing this via stdin avoids a lot of quoting issues compared to passing commands to `c4 connect`
# (imagine quoting the `confd_client` call correctly for bash->ssh->bash...)
#
# It's also necessary because `c4 connect` unconditionally consumes stdin. If didn't supply it with
# something on stdin, it would read the rest of our script as the deployment scripts are essentially
# being run via `cat deployment_script.sh | ssh nXY`
tls_ca="$(infra_jq -er '.tlsCa')"
tls_cert="$(infra_jq -er '.tlsCert')"
tls_key="$(infra_jq -er '.tlsKey')"

{
	cat <<'CONNECTEOF'
set -Eeuo pipefail

# Ensure cleanup of temporary files on exit
trap 'rm -f /root/tls_ca /root/tls_cert /root/tls_key' EXIT

# Ensure no-one can read temporary files
old_umask="$(umask)"
umask 077

cat >/root/tls_ca <<'EOF'
CONNECTEOF
	printf '%s\n' "${tls_ca}"
	cat <<'CONNECTEOF'
EOF

cat >/root/tls_cert <<'EOF'
CONNECTEOF
	printf '%s\n' "${tls_cert}"
	cat <<'CONNECTEOF'
EOF

cat >/root/tls_key <<'EOF'
CONNECTEOF
	printf '%s\n' "${tls_key}"
	cat <<'CONNECTEOF'
EOF

umask "$old_umask"

# Sadly need to ignore errors here as we currently raise a warning because the cert has URI instead of DNS/IP
# and also we sometimes get connection resets if confd reloads the cert while we're stil getting the result
confd_client cert_update ca: '"{< /root/tls_ca}"' cert: '"{< /root/tls_cert}"' key: '"{< /root/tls_key}"' || true

CONNECTEOF
} | "$C4_PATH" connect -s cos -i "$PLAY_ID"


log_step_info "TLS certificates configured"