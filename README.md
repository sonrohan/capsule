# Capsule

Capsule is a Git-compatible execution layer that records commands, logs, artifacts,
runtime metadata, and replay instructions for local development sessions.

It does not replace Git. It adds structured execution history beside a normal Git
repo under `.capsule/`.

## MVP Flow

```sh
go run . start
go run . run go test ./...
go run . finish
go run . list
go run . replay <capsule-id>
go run . ui
```

Build a binary:

```sh
go build -o capsule .
```

Then use:

```sh
./capsule start
./capsule run ./gradlew test
./capsule finish
./capsule replay <capsule-id>
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

## Replay

`capsule replay <id>` prints the exact execution history, Git linkage, artifacts,
and restored log locations.

`capsule replay <id> --rerun` reruns the recorded commands in order.

MVP replay intentionally focuses on command reproducibility and execution
visibility. It does not virtualize the full filesystem or restore a VM snapshot.
