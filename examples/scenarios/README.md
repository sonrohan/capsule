# Capsule Scenarios

These examples show when Capsule is useful and why `.capsule/` is ignored by
Git.

## Scenario 1: Local Failure Repro

You are working on a branch and a test fails locally.

```sh
./capsule ci go test ./...
```

Capsule records:

- the Git SHA and branch
- the exact command
- stdout, stderr, and combined logs
- exit code and duration
- environment metadata
- detected artifacts

Then share:

```sh
./capsule summary --last --redact
./capsule bundle --last --redact
```

What you paste into Slack or an issue:

```text
# Capsule cap_abc123
Git: f543e8709b6d7e4946a9ca9ee6abdb43a9193604 on main
Commands: 1
Artifacts: 0
Failed: go test ./...
Exit code: 1
Log: .capsule/capsules/cap_abc123/logs/001-combined.log
Replay: capsule replay cap_abc123 --rerun
Bundle: .capsule/bundles/cap_abc123-redacted.zip
```

Why this is useful:

Instead of saying "tests fail on my machine" and pasting partial logs, you send a
structured repro package with logs, command history, Git linkage, and environment
metadata.

## Scenario 2: CI Failure Artifact

In GitHub Actions:

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

Why this is useful:

When CI fails, the useful output is not only a wall of logs. The uploaded artifact
contains the command history, exit codes, environment fingerprint, logs, and any
detected test artifacts.

## Scenario 3: Bug Report Handoff

Someone reports a bug that only happens after a specific command sequence.

```sh
./capsule start
./capsule run npm install
./capsule run npm test
./capsule run npm run build
./capsule finish
./capsule summary --last --redact
./capsule bundle --last --redact
```

Why this is useful:

The receiver can inspect exactly what happened before trying to reproduce it:

```sh
./capsule replay <id>
./capsule replay <id> --rerun
```

## Scenario 4: Agent Debugging Handoff

You want an AI coding agent to debug a failure.

Instead of pasting scattered terminal output, give the agent:

- the Git SHA
- `manifest.json`
- `commands.json`
- `metadata.json`
- the combined log

Those files are all inside the Capsule bundle.

Why this is useful:

Agents work better with structured state than with prose. Capsule gives the agent
a deterministic execution record to inspect before making changes.

## Scenario 5: Repo-Specific Capture Policy

Different projects have different evidence. Keep a `capsule.json` beside the
project when the defaults are close but not quite right.

For example, a Go project that writes coverage data can add:

```json
{
  "artifacts": {
    "kinds": {
      "go-coverage": ["**/coverage.out"]
    },
    "command_filters": {
      "go:test": ["go-coverage", "junit-xml", "log"]
    }
  },
  "bundle": {
    "exclude": ["logs/*-stdout.log", "logs/*-stderr.log"]
  }
}
```

An Android project might keep APKs in local snapshots but leave them out of
shared bundles:

```json
{
  "capture": {
    "max_artifact_bytes": 52428800
  },
  "bundle": {
    "exclude": ["artifacts/**/*.apk"]
  }
}
```

## Why Not Commit `.capsule/`?

Do not commit `.capsule/` by default because it is generated execution data.

It may include:

- large logs
- generated app binaries
- screenshots
- local paths
- hostnames and usernames
- machine-specific environment metadata
- duplicate data from many local runs

Commit source, tests, docs, and workflow examples. Share Capsule bundles only
when the execution evidence is relevant.

## When Could You Commit Capsule Data?

Rarely, for a tiny curated fixture.

For example, a future `examples/fixtures/capsules/` directory might contain a
small redacted Capsule used by tests or documentation. That is different from
committing every local `.capsule/` run.
