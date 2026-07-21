#!/bin/bash
set -euo pipefail

OS="${1:-all}"

BRANCH=$(git rev-parse --abbrev-ref HEAD)
if [[ "$BRANCH" == "HEAD" ]]; then
    echo "ERROR: Cannot trigger workflow from a detached HEAD — check out a branch first" >&2
    exit 1
fi

LOCAL_SHA=$(git rev-parse HEAD)
REMOTE_SHA=$(git rev-parse "origin/${BRANCH}" 2>/dev/null) || {
    echo "ERROR: Branch '${BRANCH}' has not been pushed to origin" >&2
    exit 1
}

if [[ "$LOCAL_SHA" != "$REMOTE_SHA" ]]; then
    echo "ERROR: Local commit ${LOCAL_SHA:0:7} has not been pushed (origin/${BRANCH} is at ${REMOTE_SHA:0:7})" >&2
    exit 1
fi

echo "Triggering integration tests on ${BRANCH} (${LOCAL_SHA:0:7}), os=${OS}"
gh workflow run tests-integration.yml --ref "${BRANCH}" --field "os=${OS}"
