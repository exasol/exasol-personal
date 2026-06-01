#!/usr/bin/env bash
set -euo pipefail

docs_quality_required="false"
full_ci_required="true"

if [[ "${GITHUB_EVENT_NAME}" == "pull_request" ]]; then
  changed_files="$(git diff --name-only "origin/${GITHUB_BASE_REF}...HEAD" | sort)"

  if [[ -n "${changed_files}" ]]; then
    mapfile -t ciignore_patterns < <(grep -Ev '^[[:space:]]*(#|$)' .ciignore)
    exempt_files=""
    full_ci_files=""

    while IFS= read -r file; do
      exempt="false"
      for pattern in "${ciignore_patterns[@]}"; do
        if [[ "${file}" == ${pattern} ]]; then
          exempt="true"
          break
        fi
      done

      if [[ "${exempt}" == "true" ]]; then
        exempt_files+="${file}"$'\n'
      else
        full_ci_files+="${file}"$'\n'
      fi
    done <<< "${changed_files}"

    if [[ -n "${exempt_files}" ]]; then
      docs_quality_required="true"
    fi
    if [[ -z "${full_ci_files}" ]]; then
      full_ci_required="false"
    fi

    printf 'Full CI exempt files:\n%s\n' "${exempt_files:-none}"
    printf 'Full CI files:\n%s\n' "${full_ci_files:-none}"
  else
    echo "No changed files found; requiring full CI."
  fi
else
  echo "Main branch push: full CI required."
fi

{
  echo "docs_quality_required=${docs_quality_required}"
  echo "full_ci_required=${full_ci_required}"
} >> "${GITHUB_OUTPUT}"

echo "Docs quality required: ${docs_quality_required}"
echo "Full CI required: ${full_ci_required}"
