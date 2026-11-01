#!/usr/bin/env bash
# Hash protected paths so the agent cannot silently edit acceptance tests or drill code.
set -euo pipefail
cd "$(dirname "$0")/.."
PROTECTED=(acceptance internal/controller internal/drills scripts/manifest.sh)
MANIFEST=".manifest.sha256"

list() {
  find "${PROTECTED[@]}" -type f ! -name '.gitkeep' 2>/dev/null | LC_ALL=C sort | xargs -r sha256sum
}

case "${1:-}" in
  create)
    list >"$MANIFEST"
    echo "manifest written: $MANIFEST ($(wc -l <"$MANIFEST") files)"
    ;;
  verify)
    [ -f "$MANIFEST" ] || { echo "no manifest; run: bash scripts/manifest.sh create" >&2; exit 1; }
    if diff -u "$MANIFEST" <(list) >/dev/null; then
      echo "manifest OK"
    else
      echo "PROTECTED PATH MISMATCH" >&2
      diff -u "$MANIFEST" <(list) >&2 || true
      exit 1
    fi
    ;;
  *)
    echo "usage: manifest.sh create|verify" >&2
    exit 2
    ;;
esac
