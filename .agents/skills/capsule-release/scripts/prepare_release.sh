#!/bin/sh
set -eu

usage() {
  cat <<'USAGE'
Usage: prepare_release.sh [--dry-run] [--push] <version>

Examples:
  prepare_release.sh v0.2.0
  prepare_release.sh --dry-run 0.2.0
  prepare_release.sh --push v0.2.0
USAGE
}

DRY_RUN=0
PUSH=0

while [ "$#" -gt 0 ]; do
  case "$1" in
    --dry-run)
      DRY_RUN=1
      shift
      ;;
    --push)
      PUSH=1
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    --*)
      echo "unknown option: $1" >&2
      usage >&2
      exit 2
      ;;
    *)
      break
      ;;
  esac
done

if [ "$#" -ne 1 ]; then
  usage >&2
  exit 2
fi

VERSION_INPUT="$1"
VERSION="${VERSION_INPUT#v}"
TAG="v$VERSION"

case "$VERSION" in
  *[!0-9.]*|*..*|.*|*.)
    echo "version must be SemVer like 0.2.0 or v0.2.0" >&2
    exit 2
    ;;
esac

old_ifs="$IFS"
IFS=.
set -- $VERSION
IFS="$old_ifs"

if [ "$#" -ne 3 ] || [ -z "$1" ] || [ -z "$2" ] || [ -z "$3" ]; then
  echo "version must be SemVer like 0.2.0 or v0.2.0" >&2
  exit 2
fi

REPO_ROOT="$(git rev-parse --show-toplevel)"
cd "$REPO_ROOT"

if [ ! -f VERSION ] || [ ! -f main.go ] || [ ! -f .goreleaser.yaml ]; then
  echo "run this from the Capsule repository; expected VERSION, main.go, and .goreleaser.yaml" >&2
  exit 1
fi

if git rev-parse -q --verify "refs/tags/$TAG" >/dev/null; then
  echo "tag already exists locally: $TAG" >&2
  exit 1
fi

current_version="$(tr -d '[:space:]' < VERSION)"
if [ "$current_version" = "$VERSION" ]; then
  echo "VERSION is already $VERSION" >&2
  exit 1
fi

if [ "$DRY_RUN" -eq 1 ]; then
  echo "Would prepare release $TAG:"
  echo "  update VERSION: $current_version -> $VERSION"
  echo "  update main.go fallback version"
  echo "  run validation"
  echo "  commit Release $TAG"
  echo "  tag $TAG"
  if [ "$PUSH" -eq 1 ]; then
    echo "  push main and $TAG"
  fi
  exit 0
fi

if [ -n "$(git status --porcelain)" ]; then
  echo "working tree is dirty; commit or stash changes before preparing a release" >&2
  git status --short
  exit 1
fi

if git ls-remote --exit-code --tags origin "refs/tags/$TAG" >/dev/null 2>&1; then
  echo "tag already exists on origin: $TAG" >&2
  exit 1
fi

printf '%s\n' "$VERSION" > VERSION

tmp_file="$(mktemp)"
awk -v version="$VERSION" '
  /^[[:space:]]*version = "/ && !done {
    sub(/"[^"]*"/, "\"" version "\"")
    done = 1
  }
  { print }
  END {
    if (!done) {
      exit 3
    }
  }
' main.go > "$tmp_file" || {
  rm -f "$tmp_file"
  echo "failed to update main.go fallback version" >&2
  exit 1
}
mv "$tmp_file" main.go

gofmt -w main.go
sh -n install.sh
go test ./...
go run github.com/goreleaser/goreleaser/v2@latest check

git diff -- VERSION main.go
git add VERSION main.go
git commit -m "Release $TAG"
git tag -a "$TAG" -m "Release $TAG"

if [ "$PUSH" -eq 1 ]; then
  branch="$(git branch --show-current)"
  git push origin "$branch"
  git push origin "$TAG"
fi

echo "Prepared release $TAG"
