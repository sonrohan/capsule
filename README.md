# Capsule

Capsule is a Git-compatible execution layer that records commands, logs, artifacts,
runtime metadata, and replay instructions for local development sessions.

It does not replace Git. It adds structured execution history beside a normal Git
repo under `.capsule/`.

## Should `.capsule/` Be Committed?

Usually, no.

`.capsule/` is runtime output, like `build/`, `coverage/`, `.gradle/`, or test
logs. It can contain large logs, generated binaries, screenshots, local machine
paths, hostnames, usernames, and environment details. Keeping it out of normal Git
history preserves clean source commits and avoids accidentally publishing local or
sensitive execution data.

The intended sharing unit is a Capsule bundle:

```sh
capsule ci go test ./...
capsule bundle --last
```

That creates:

```text
.capsule/bundles/<id>.zip
```

Attach that zip to CI artifacts, GitHub issues, Slack threads, or bug reports.
The repo stores source history. Capsule bundles store execution evidence.

## Install for Local Development

Prerequisite: Go 1.22 or newer. From this repo:

```sh
make install
capsule --help
```

If `capsule` is not found after install, add Go's bin directory to your shell
profile and reload the shell:

```sh
echo 'export PATH="$HOME/go/bin:$PATH"' >> ~/.zshrc
exec zsh
```

Without `make`, use Go directly:

```sh
go install .
```

## Quick Start

```sh
capsule start
capsule run go test ./...
capsule finish
capsule list
capsule replay <capsule-id>
capsule summary --last
capsule bundle --last
capsule ui
```

For one-shot capture:

```sh
capsule ci go test ./...
```

To build a repo-local binary instead of installing:

```sh
go build -o capsule .
```

## What Capsule Stores

Finished sessions are stored at:

```text
.capsule/capsules/<id>/
  manifest.json
  commands.json
  metadata.json
  logs/
  artifacts/
```

Each manifest includes:

- Git SHA, branch, and dirty state
- OS, architecture, host, shell, and runtime versions
- Commands, timings, exit codes, and log paths
- Detected artifacts such as JUnit XML, APKs, screenshots, and logs

Bundles are written to:

```text
.capsule/bundles/<id>.zip
```

## Replay

`capsule replay <id>` prints the exact execution history, Git linkage, artifacts,
and restored log locations.

`capsule replay <id> --rerun` reruns the recorded commands in order.

`capsule summary --last` prints a copy-pasteable repro summary.

`capsule bundle --last` creates a zip file that can be attached to CI artifacts,
GitHub issues, or Slack threads.

MVP replay intentionally focuses on command reproducibility and execution
visibility. It does not virtualize the full filesystem or restore a VM snapshot.

## Examples

See [examples/scenarios](examples/scenarios) for concrete workflows:

- local failing test reproduction
- CI artifact capture
- bug report handoff
- agent debugging handoff
