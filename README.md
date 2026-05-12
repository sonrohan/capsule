# Capsule

Capsule is a flight recorder for debugging handoffs.

It captures what ran, what failed, which Git state produced it, and which logs
or artifacts matter, then packages that evidence for another developer, a CI
artifact, or a coding agent.

Git stores source history. Capsule stores execution history.

## The 30-Second Loop

When a local run fails:

```sh
capsule ci go test ./...
capsule summary --last
capsule agent --last
capsule bundle --last
```

What you get back is a portable repro package instead of pasted terminal output:

```text
# Capsule cap_x
Git: 973149d on main
Commands: 1
Artifacts: 0
Failed: go test ./...
Exit code: 1
Log: .capsule/capsules/cap_x/logs/001-combined.log
Replay: capsule replay cap_x --rerun
```

`capsule agent --last` prints a ready-to-paste debugging brief that points an
agent at `manifest.json`, `commands.json`, `metadata.json`, and the combined
log before it starts proposing fixes.

## Where It Helps

- Local failures that need a clean handoff.
- CI jobs where logs alone are not enough.
- PRs or issues that need a reproducible command trail.
- Agent workflows where structured evidence is better than prose.
- Builds, tests, lint runs, screenshots, and package outputs that should travel
  with the failure.

Capsule is intentionally lightweight. It does not snapshot the whole machine or
recreate dependencies. Replay reruns the recorded commands against the current
working tree unless you check out the captured Git SHA first.

## Install

macOS:

```sh
curl -fsSL https://raw.githubusercontent.com/sonrohan/capsule/main/install.sh | sh
capsule --version
```

If `capsule` is not on PATH after install:

```sh
echo 'export PATH="$HOME/.local/bin:$PATH"' >> ~/.zshrc
exec zsh
```

Capsule currently publishes prebuilt binaries for macOS only.

Build from source with Go 1.22 or newer:

```sh
go install github.com/sonrohan/capsule@latest
```

From this repository:

```sh
make install
```

## Core Workflow

One-shot capture:

```sh
capsule ci go test ./...
```

Multi-step capture:

```sh
capsule start
capsule run npm install
capsule run npm test
capsule run npm run build
capsule finish
```

Inspect and replay:

```sh
capsule list
capsule summary --last
capsule agent --last
capsule replay <capsule-id>
capsule replay <capsule-id> --rerun
```

Share and receive:

```sh
capsule bundle --last
capsule import .capsule/bundles/<capsule-id>.zip
```

Privacy-aware sharing:

```sh
capsule summary --last --redact
capsule agent --last --redact
capsule bundle --last --redact
```

Open the local UI:

```sh
capsule ui
capsule ui --port 3001
```

The UI highlights the failed command first, shows an inline combined-log preview,
links to the bundle, and exposes a copyable agent briefing.

## Commands

```text
capsule start
capsule run <command> [args...]
capsule finish
capsule ci <command> [args...]
capsule list
capsule summary <capsule-id|--last> [--redact]
capsule agent <capsule-id|--last> [--redact]
capsule bundle <capsule-id|--last> [--redact] [--include glob] [--exclude glob] [--no-artifacts]
capsule import <bundle.zip>
capsule replay <capsule-id> [--rerun]
capsule ui [--port 3000]
capsule version
```

## What Capsule Stores

A finished snapshot lives at:

```text
.capsule/capsules/<id>/
  manifest.json
  commands.json
  metadata.json
  logs/
  artifacts/
```

It includes:

- Git SHA, branch, repository path, and dirty-state metadata.
- Runtime metadata such as OS, architecture, shell, working directory, and
  discovered tool versions.
- Command arguments, timing, exit code, and per-command log paths.
- Separate stdout, stderr, and combined logs.
- Detected artifacts copied into the snapshot.

Bundles are written to:

```text
.capsule/bundles/<id>.zip
```

Imported bundles are restored under `.capsule/capsules/<id>/`.

## Artifact Detection

Capsule scans the repository after each command and copies recognized outputs
into the session snapshot. It skips `.git`, `.capsule`, `node_modules`, and
`.gradle`.

Recognized types include:

- Android APKs: `*.apk`
- iOS packages: `*.ipa`
- JUnit XML from common test result paths
- Android lint reports
- Logs: `*.log`
- Screenshots and snapshots: `*.png`, `*.jpg`, `*.jpeg` when the path indicates
  screenshot or snapshot output

For Gradle commands, Capsule narrows capture based on the task name so test
tasks keep test evidence, lint tasks keep lint evidence, and assemble or bundle
tasks keep package outputs.

## Repository Configuration

Capsule works without configuration. To customize what is captured, redacted, or
bundled for a repository, add `capsule.json` at the repository root.

User config extends the built-in defaults for most lists. That means you can add
an artifact kind or extra exclude without restating the defaults. Explicit
`include` lists are narrower by design: if an include list is present, only
matching paths are eligible.

```json
{
  "capture": {
    "scan_roots": ["."],
    "exclude": ["dist/cache/**"],
    "max_artifact_bytes": 52428800
  },
  "artifacts": {
    "enabled": true,
    "kinds": {
      "go-coverage": ["**/coverage.out"],
      "playwright-report": ["**/playwright-report/**"]
    },
    "exclude": ["**/*.mp4"],
    "command_filters": {
      "go:test": ["go-coverage", "junit-xml", "log"]
    }
  },
  "redaction": {
    "defaults": true,
    "literals": ["AcmeInternal"],
    "replace": [
      { "pattern": "tenant_[A-Za-z0-9]+", "with": "[TENANT]" }
    ],
    "allow": ["public-demo-user"],
    "files": {
      "exclude": ["artifacts/**/*.html"]
    }
  },
  "bundle": {
    "exclude": ["artifacts/**/*.apk", "logs/*-stdout.log", "logs/*-stderr.log"]
  }
}
```

Config areas:

- `capture.scan_roots`: directories Capsule scans after each command.
- `capture.include` / `capture.exclude`: global path filters for captured
  outputs.
- `capture.max_artifact_bytes`: skip artifacts larger than this many bytes.
- `artifacts.enabled`: set to `false` to disable artifact copying.
- `artifacts.kinds`: artifact kind names mapped to glob patterns.
- `artifacts.include` / `artifacts.exclude`: filters applied after global
  capture filters.
- `artifacts.command_filters`: command keywords mapped to allowed artifact
  kinds. A key like `gradle:test` matches commands containing both words.
- `redaction.defaults`: keep or disable built-in username, hostname, home path,
  working directory, repository path, and common token redaction.
- `redaction.literals`: exact strings replaced with `[REDACTED]`.
- `redaction.replace`: regular expressions and replacement text.
- `redaction.allow`: exact default/literal redaction strings to leave visible.
- `redaction.files.include` / `redaction.files.exclude`: which files are
  text-redacted inside redacted bundles.
- `bundle.include` / `bundle.exclude`: files written to bundle zips.

Bundle flags can override packaging for one run:

```sh
capsule bundle --last --redact --no-artifacts
capsule bundle --last --exclude 'artifacts/**/*.apk'
capsule bundle --last --include 'manifest.json' --include 'logs/*-combined.log'
```

## CI Example

```yaml
name: test

on:
  pull_request:
  push:
    branches: [main]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.22'
      - run: go build -o capsule .
      - run: ./capsule ci go test ./...
      - if: always()
        run: ./capsule bundle --last --redact || true
      - if: always()
        uses: actions/upload-artifact@v4
        with:
          name: capsule-repro
          path: .capsule/bundles/*.zip
```

## Sharing Policy

Do not commit `.capsule/` by default.

It can contain large logs, generated binaries, screenshots, local paths,
hostnames, usernames, and environment metadata. Commit only small curated
fixtures that are intentionally part of documentation or tests.

For external sharing, prefer redacted summaries and redacted bundles.

## Examples

- [examples/scenarios/README.md](/Users/rohan/repos/capsule/examples/scenarios/README.md)
- [examples/android/README.md](/Users/rohan/repos/capsule/examples/android/README.md)
- [examples/failing-go-test/README.md](/Users/rohan/repos/capsule/examples/failing-go-test/README.md)

## Development

Useful commands when working on Capsule itself:

```sh
make build
make test
make install
/bin/zsh -lc 'PATH=/usr/local/go/bin:$PATH go test ./...'
```
