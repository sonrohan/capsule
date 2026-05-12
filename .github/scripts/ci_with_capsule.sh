#!/bin/sh
set -u

mkdir -p .capsule/ci-bin
go build -o .capsule/ci-bin/capsule .

status=0

.capsule/ci-bin/capsule start

if .capsule/ci-bin/capsule run make build; then
  :
else
  status=$?
fi

if [ "$status" -eq 0 ]; then
  if .capsule/ci-bin/capsule run make test; then
    :
  else
    status=$?
  fi
fi

.capsule/ci-bin/capsule finish
.capsule/ci-bin/capsule summary --last --redact
.capsule/ci-bin/capsule bundle --last --redact

exit "$status"
