#!/usr/bin/env bash
# Validates Agent Skills format: folder name == frontmatter name, name charset, description present.
set -euo pipefail
cd "$(dirname "$0")/.."
bad=0
for dir in .agents/skills/*/; do
  name="$(basename "$dir")"
  f="$dir/SKILL.md"
  [ -f "$f" ] || { echo "missing SKILL.md: $dir"; bad=1; continue; }
  fm_name="$(awk '/^---$/{c++;next} c==1 && /^name:/{sub(/^name:[ ]*/,"");print;exit}' "$f")"
  fm_desc="$(awk '/^---$/{c++;next} c==1 && /^description:/{sub(/^description:[ ]*/,"");print;exit}' "$f")"
  [ "$fm_name" = "$name" ] || { echo "name mismatch in $f: '$fm_name' vs folder '$name'"; bad=1; }
  echo "$name" | grep -Eq '^[a-z0-9]+(-[a-z0-9]+)*$' && [ ${#name} -le 64 ] || { echo "bad name: $name"; bad=1; }
  [ -n "$fm_desc" ] && [ ${#fm_desc} -le 1024 ] || { echo "bad description in $f"; bad=1; }
done
[ $bad -eq 0 ] && echo "skills OK ($(ls -d .agents/skills/*/ | wc -l) skills)" || exit 1
