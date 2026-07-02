#!/bin/bash
set -euo pipefail

BRANCH=$(git rev-parse --abbrev-ref HEAD)
if [[ "$BRANCH" == "HEAD" ]]; then
    echo "ERROR: Cannot trigger a CI run from a detached HEAD — check out a branch first"
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

EXISTING_RUN_IDS=$(gh run list --workflow=ci.yml --branch="${BRANCH}" --json databaseId --jq '.[].databaseId')

echo "Triggering CI on ${BRANCH} (${LOCAL_SHA:0:7})"
gh workflow run ci.yml --ref "${BRANCH}"

echo "Waiting for the new CI run to appear"

RUN_ID=""
for _ in $(seq 1 30); do
    RUN_ID=$(gh run list --workflow=ci.yml --branch="${BRANCH}" \
        --json databaseId,headSha \
        --jq "[.[] | select(.headSha == \"${LOCAL_SHA}\")][0].databaseId" 2>/dev/null || true)
    if [[ -n "${RUN_ID}" && "${RUN_ID}" != "null" ]] && ! grep -qx "${RUN_ID}" <<<"${EXISTING_RUN_IDS}"; then
        break
    fi
    RUN_ID=""
    echo "New CI run not found yet, retrying..."
    sleep 10
done

if [[ -z "${RUN_ID}" ]]; then
    echo "ERROR: Could not find the newly triggered CI run for ${LOCAL_SHA:0:7} on ${BRANCH}"
    exit 1
fi

echo "Watching CI run ${RUN_ID} (this includes lint + the build job for all supported platforms)"
gh run watch "${RUN_ID}" --exit-status

DOWNLOAD_DIR="ci-downloads"
mkdir -p "${DOWNLOAD_DIR}"

echo "Downloading built binaries to ${DOWNLOAD_DIR}/"
gh run download "${RUN_ID}" --dir "${DOWNLOAD_DIR}" --pattern "exasol-*"

echo "Done. Binaries available in ${DOWNLOAD_DIR}/"
