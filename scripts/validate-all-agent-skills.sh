#!/usr/bin/env sh
set -eu

repo_root="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
cd "$repo_root"

if [ ! -f skills/openbrief/SKILL.md ]; then
  echo "expected core OpenBrief skill" >&2
  exit 1
fi

skill_list=$(mktemp "${TMPDIR:-/tmp}/openbrief-agent-skills.XXXXXX")
trap 'rm -f "$skill_list"' EXIT HUP INT TERM

find skills -mindepth 2 -maxdepth 2 -name SKILL.md -type f | sort >"$skill_list"

count=0
while IFS= read -r skill_file; do
  [ -n "$skill_file" ] || continue
  skill_dir=${skill_file%/SKILL.md}
  ./scripts/validate-agent-skill.sh "$skill_dir"
  count=$((count + 1))
done <"$skill_list"

if [ "$count" -lt 1 ]; then
  echo "expected core OpenBrief skill" >&2
  exit 1
fi

printf 'validated %s agent skills\n' "$count"
