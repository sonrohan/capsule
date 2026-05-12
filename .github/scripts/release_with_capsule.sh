#!/bin/sh
set -eu

if [ -z "${GITHUB_REF_NAME:-}" ]; then
  echo "GITHUB_REF_NAME is required" >&2
  exit 1
fi

if ! command -v goreleaser >/dev/null 2>&1; then
  echo "goreleaser is required" >&2
  exit 1
fi

if ! command -v gh >/dev/null 2>&1; then
  echo "gh is required" >&2
  exit 1
fi

mkdir -p .capsule/release-bin dist
go build -o .capsule/release-bin/capsule .

.capsule/release-bin/capsule ci goreleaser release --clean

bundle_path="$(find .capsule/bundles -type f -name '*.zip' -print | sort | tail -n 1)"
if [ -z "$bundle_path" ]; then
  echo "no Capsule release evidence bundle found" >&2
  exit 1
fi

evidence_path="dist/capsule_${GITHUB_REF_NAME}_release-evidence.zip"
cp "$bundle_path" "$evidence_path"

gh release upload "$GITHUB_REF_NAME" "$evidence_path#Release evidence captured with Capsule" --clobber
