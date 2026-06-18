#!/bin/bash
set -euo pipefail

DOWNLOAD_DIR="${1:-ci-downloads}"
WORKFLOW="ci.yml"

BRANCH=$(git rev-parse --abbrev-ref HEAD)
if [[ "$BRANCH" == "HEAD" ]]; then
    echo "ERROR: Cannot determine CI run from a detached HEAD — check out a branch first"
    exit 1
fi

echo "Looking for latest successful ${WORKFLOW} run on '${BRANCH}'..."

RUN_ID=$(gh run list \
    --workflow="${WORKFLOW}" \
    --branch="${BRANCH}" \
    --status=success \
    --limit=1 \
    --json=databaseId \
    --jq='.[0].databaseId')

if [[ -z "$RUN_ID" || "$RUN_ID" == "null" ]]; then
    echo "ERROR: No successful ${WORKFLOW} run found for branch '${BRANCH}'"
    exit 1
fi

echo "Downloading artifacts from run ${RUN_ID} into '${DOWNLOAD_DIR}/'..."
mkdir -p "${DOWNLOAD_DIR}"
gh run download "${RUN_ID}" --dir "${DOWNLOAD_DIR}"
echo "Done."
