# Agent Guide

This repository contains Capsule, a small Go CLI for recording replayable
execution sessions and packaging logs/artifacts for debugging handoffs.

## Build And Test

- Run the full test suite with `go test ./...` or `make test`.
- Build the local CLI with `make build`; it writes `./capsule`.
- Install from the repo with `make install`.
- Go 1.22 or newer is required.

Before finishing a code change, run `gofmt -w` on touched Go files and then
`go test ./...`.

## Code Layout

- `main.go`: CLI dispatch, usage text, version printing.
- `models.go`: shared session, artifact, config, and redaction types.
- `commands.go`: command handlers for `start`, `run`, `finish`, `ci`, `summary`,
  `agent`, `bundle`, `import`, `replay`, and `list`.
- `config.go`: `capsule.json` defaults, merging, and glob matching.
- `artifacts.go`: artifact detection, command filters, artifact merging.
- `bundle.go`: bundle creation and import.
- `redaction.go`: redaction policy and text replacement.
- `metadata.go`: Git and runtime environment collection.
- `snapshot.go`: snapshot paths, JSON helpers, file copying, IDs.
- `store.go`: active session and finished capsule loading.
- `summary.go`: summary and agent briefing text.
- `ui.go`: local HTTP UI and template.

Keep this split roughly intact. Prefer adding behavior to the file that owns the
concept rather than moving unrelated code around.

## Behavior Notes

- No `capsule.json` should preserve the built-in defaults.
- Redacted summaries and bundles should fail clearly on invalid redaction regexes.
- Do not commit generated `.capsule/` data except curated fixtures already
  allowed by `.gitignore`.
- The committed examples under `examples/` are documentation fixtures. Update
  them when user-facing workflow or config behavior changes.
- Keep dependencies minimal. The CLI currently uses only the Go standard
  library.

## Release Notes

Release work has its own local skill under `.agents/skills/capsule-release/`.
Use that workflow for version bumps, tags, GoReleaser checks, or publishing.
