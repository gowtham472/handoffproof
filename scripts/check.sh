#!/usr/bin/env bash
# Quality gate. Run before saying anything is done.
set -euo pipefail
cd "$(dirname "$0")/.."

if [ -f go.mod ] && command -v go >/dev/null 2>&1; then
  echo "== gofmt"
  unformatted="$(gofmt -l . || true)"
  if [ -n "$unformatted" ]; then echo "needs gofmt:"; echo "$unformatted"; exit 1; fi
  echo "== go vet"
  go vet ./...
  echo "== go test"
  go test ./... -timeout 15m
else
  echo "skip go checks (no go.mod or go not installed)"
fi

echo "== skills"
bash scripts/check-skills.sh
echo "== protected paths"
if [ -f .manifest.sha256 ]; then bash scripts/manifest.sh verify; else echo "no manifest yet (create one after acceptance tests exist)"; fi
echo "check OK"
