#!/bin/bash
set -euo pipefail

OS="${1:-all}"
SUITE="${2:-all}"
DEBUG_ARTIFACTS="${3:-false}"

BRANCH=$(git rev-parse --abbrev-ref HEAD)
if [[ "$BRANCH" == "HEAD" ]]; then
    echo "ERROR: Cannot trigger workflow from a detached HEAD — check out a branch first"
    exit 1
fi

LOCAL_SHA=$(git rev-parse HEAD)
REMOTE_SHA=$(git rev-parse "origin/${BRANCH}" 2>/dev/null) || {
    echo "ERROR: Branch '${BRANCH}' has not been pushed to origin"
    exit 1
}

if [[ "$LOCAL_SHA" != "$REMOTE_SHA" ]]; then
    echo "ERROR: Local commit ${LOCAL_SHA:0:7} has not been pushed (origin/${BRANCH} is at ${REMOTE_SHA:0:7})"
    exit 1
fi

if [[ "${DEBUG_ARTIFACTS}" != "true" ]]; then
    echo "Triggering deployment tests on ${BRANCH} (${LOCAL_SHA:0:7}), suite=${SUITE}, os=${OS}"
    gh workflow run tests-deployment.yml --ref "${BRANCH}" \
        --field "suite=${SUITE}" \
        --field "os=${OS}" \
        --field "debug_artifacts=${DEBUG_ARTIFACTS}"
    exit 0
fi

# DEBUG_ARTIFACTS=true: wait for the run to finish and download any failure artifacts.
EXISTING_RUN_IDS=$(gh run list --workflow=tests-deployment.yml --branch="${BRANCH}" --json databaseId --jq '.[].databaseId')

echo "Triggering deployment tests on ${BRANCH} (${LOCAL_SHA:0:7}), suite=${SUITE}, os=${OS}, debug_artifacts=true"
gh workflow run tests-deployment.yml --ref "${BRANCH}" \
    --field "suite=${SUITE}" \
    --field "os=${OS}" \
    --field "debug_artifacts=true"

echo "Waiting for the new run to appear"

RUN_ID=""
for _ in $(seq 1 30); do
    RUN_ID=$(gh run list --workflow=tests-deployment.yml --branch="${BRANCH}" \
        --json databaseId,headSha \
        --jq "[.[] | select(.headSha == \"${LOCAL_SHA}\")][0].databaseId" 2>/dev/null || true)
    if [[ -n "${RUN_ID}" && "${RUN_ID}" != "null" ]] && ! grep -qx "${RUN_ID}" <<<"${EXISTING_RUN_IDS}"; then
        break
    fi
    RUN_ID=""
    echo "New run not found yet, retrying..."
    sleep 10
done

if [[ -z "${RUN_ID}" ]]; then
    echo "ERROR: Could not find the newly triggered run for ${LOCAL_SHA:0:7} on ${BRANCH}"
    exit 1
fi

echo "Watching deployment tests run ${RUN_ID}"
# Don't fail the script on test failures — we still want to download the
# debug artifacts that a failure produces.
gh run watch "${RUN_ID}" || true

DOWNLOAD_DIR="ci-downloads"
mkdir -p "${DOWNLOAD_DIR}"

if gh run download "${RUN_ID}" --dir "${DOWNLOAD_DIR}" --pattern "deployment-test-failures-*" 2>/dev/null; then
    echo "Debug artifacts downloaded to ${DOWNLOAD_DIR}/"

    while IFS= read -r -d '' archive; do
        echo "Decompressing $(basename "${archive}")"
        tar -xzf "${archive}" -C "$(dirname "${archive}")"
    done < <(find "${DOWNLOAD_DIR}" -name "*.tar.gz" -print0)
else
    echo "No debug artifacts were produced for this run (no failing tests, most likely)"
fi
