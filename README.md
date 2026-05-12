# Capsule

Capsule records command execution in a Git repository so failures can be
inspected, shared, and rerun with the surrounding evidence intact.

It does not replace Git, CI, test frameworks, or containers. Git stores source
history. Capsule stores execution history: commands, logs, exit codes, runtime
metadata, detected artifacts, and replay instructions.

Capsule writes runtime data under `.capsule/` in the repository where commands
are run.

## What It Is Useful For

Use Capsule when the important question is not only "what changed?" but also
"what exactly ran, what happened, and what evidence was produced?"

Common uses:

- Capturing a local failing test with full stdout, stderr, exit code, Git SHA,
  branch, dirty state, and environment metadata.
- Uploading CI failure evidence as a structured artifact instead of relying only
  on console logs.
- Handing off a bug report with the exact command sequence and generated files.
- Giving another developer or coding agent a reproducible execution record before
  they start debugging.
- Preserving build, test, lint, screenshot, and package artifacts that explain a
  failure.

Capsule is intentionally lightweight. Replay can rerun recorded commands, but it
does not virtualize the filesystem, restore dependencies, or create a machine
snapshot. If a command depends on external services, local credentials, caches,
or uncommitted files, those dependencies still need to exist when rerunning.

## Install

macOS:

```sh
curl -fsSL https://raw.githubusercontent.com/rohan/capsule/main/install.sh | sh
capsule --version
capsule --help
```

If `capsule` is not found after install, add Capsule's install directory to your
shell profile and reload the shell:

```sh
echo 'export PATH="$HOME/.local/bin:$PATH"' >> ~/.zshrc
exec zsh
```

Capsule currently publishes prebuilt binaries for macOS only. Linux and Windows
support are welcome via pull request.

### Build from Source

Requires Go 1.22 or newer.

```sh
go install github.com/rohan/capsule@latest
```

From this repository:

```sh
make install
```

## Quick Start

For a one-command capture, use `ci`. It starts a session, runs the command,
finishes the session, prints a summary, and creates a bundle.

```sh
capsule ci go test ./...
```

For a multi-command workflow:

```sh
capsule start
capsule run go test ./...
capsule run go build ./...
capsule finish
capsule summary --last
capsule bundle --last
```

Inspect or rerun a finished Capsule:

```sh
capsule list
capsule replay <capsule-id>
capsule replay <capsule-id> --rerun
```

Open the local web UI:

```sh
capsule ui
capsule ui --port 3001
```

## Commands

```text
capsule start
```

Starts a new active session in `.capsule/sessions/<id>/` and records Git and
environment metadata.

```text
capsule run <command> [args...]
```

Runs a command inside the active session. Capsule streams stdout and stderr to
the terminal while also saving per-command logs, duration, exit code, and
detected artifacts.

```text
capsule finish
```

Finishes the active session and writes a snapshot to `.capsule/capsules/<id>/`.

```text
capsule ci <command> [args...]
```

Runs a one-shot capture for CI or local repros. It creates a session, runs the
command, finishes the snapshot, prints a summary, creates a bundle, and exits
with the wrapped command's exit code if the command fails.

```text
capsule list
```

Lists finished Capsules with Git SHA, branch, command count, and artifact count.

```text
capsule replay <capsule-id>
capsule replay <capsule-id> --rerun
```

Prints the recorded Git linkage, commands, artifacts, and log locations.
`--rerun` executes the recorded commands again in order in the current working
tree.

```text
capsule summary <capsule-id|--last>
```

Prints a compact repro summary suitable for a bug report, issue, pull request,
or chat thread.

```text
capsule bundle <capsule-id|--last>
```

Creates `.capsule/bundles/<id>.zip` containing the finished snapshot.

```text
capsule ui [--port 3000]
```

Serves a local read-only browser view of finished Capsules at
`http://127.0.0.1:<port>`.

```text
capsule version
capsule --version
```

Prints the installed version and build metadata when available.

## What Capsule Stores

A finished Capsule snapshot is stored at:

```text
.capsule/capsules/<id>/
  manifest.json
  commands.json
  metadata.json
  logs/
  artifacts/
```

The snapshot includes:

- Git SHA, branch, repository path, and dirty working tree status.
- OS, architecture, hostname, user, shell, working directory, Capsule version,
  Go version, and detected runtime tool versions.
- Command arguments, display command, start time, finish time, duration, exit
  code, and log paths.
- Separate stdout, stderr, and combined logs for each command.
- Detected artifacts copied into the snapshot.

Bundles are written to:

```text
.capsule/bundles/<id>.zip
```

The zip contains the finished Capsule under `capsule/<id>/`.

## Artifact Detection

Capsule scans the repository after each command and copies recognized files into
the session snapshot. It skips `.git`, `.capsule`, `node_modules`, and `.gradle`.

Recognized artifact types include:

- Android APKs: `*.apk`
- iOS packages: `*.ipa`
- JUnit XML from common test result paths
- Android lint reports
- Logs: `*.log`
- Screenshots and snapshots: `*.png`, `*.jpg`, `*.jpeg` when the path indicates
  screenshot or snapshot output

For Gradle commands, Capsule narrows artifact capture based on the task name:
test tasks capture test results and logs, lint tasks capture lint reports and
logs, and assemble or bundle tasks capture app packages and logs.

## Sharing Policy

Do not commit `.capsule/` by default.

`.capsule/` is generated runtime output. It can contain large logs, generated
binaries, screenshots, local paths, hostnames, usernames, and environment
details. Keeping it out of normal Git history keeps source commits clean and
reduces the chance of publishing local or sensitive execution data.

Share a bundle when the execution evidence is useful:

```sh
capsule ci go test ./...
capsule bundle --last
```

Then attach:

```text
.capsule/bundles/<id>.zip
```

Use bundles for CI artifacts, GitHub issues, pull requests, Slack threads, or bug
reports. Commit only small, curated Capsule fixtures when they are intentionally
part of examples or tests.

## Development

Useful commands for working on Capsule itself:

```sh
make build
make test
make install
go test ./...
```

The Makefile injects version metadata from `VERSION`, the current Git commit,
and the UTC build time when building or installing.

## Examples

See [examples/scenarios](examples/scenarios) for workflow examples:

- local failing test reproduction
- CI artifact capture
- bug report handoff
- agent debugging handoff

See [examples/android](examples/android) for a small Android project with a
curated Capsule snapshot.
