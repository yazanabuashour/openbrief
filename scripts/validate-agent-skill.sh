#!/usr/bin/env bash
set -euo pipefail

skill_dir="${1:-skills/openbrief}"
skill_file="${skill_dir}/SKILL.md"

test -f "${skill_file}"

file_count="$(find "${skill_dir}" -maxdepth 1 -type f | wc -l | tr -d ' ')"
if [ "${file_count}" != "1" ]; then
  echo "skill payload must contain exactly SKILL.md" >&2
  exit 1
fi

grep -q "openbrief config" "${skill_file}"
grep -q "openbrief brief" "${skill_file}"
grep -q "OPENBRIEF_DATABASE_PATH" "${skill_file}"

for forbidden in \
  "OPENBRIEF_DATA_DIR" \
  "brief-fetch.ts" \
  "BRIEF_PAYWALL_POLICY" \
  "BRIEF_SOURCES" \
  "home-openclaw" \
  "/Volumes/" \
  "/Users/"; do
  if grep -q "${forbidden}" "${skill_file}"; then
    echo "forbidden skill text: ${forbidden}" >&2
    exit 1
  fi
done
