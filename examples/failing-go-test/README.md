# Failing Go Capsule Sample

This sample is intentionally broken so the first Capsule walkthrough is a real
failure handoff instead of a green build.

## Project

The package has one passing test and one deterministic failing test:

```text
TestMultiplyByTwo
TestIntentionalFailure
```

The failing assertion is in [math_test.go](/Users/rohan/repos/capsule/examples/failing-go-test/math_test.go).

## Curated Capsule

Published Capsule ID:

```text
cap_12a03ebbc337
```

Location:

```text
examples/failing-go-test/.capsule/capsules/cap_12a03ebbc337/
```

This committed fixture was imported from a redacted bundle, so local host and
path data are removed while the failure evidence stays intact.

This directory includes a small `capsule.json` showing how a Go repo can add a
custom `go-coverage` artifact kind and omit separate stdout/stderr logs from
shared bundles while keeping the combined log.

## Recorded Command

The snapshot captured one command:

```sh
../../capsule ci go test ./...
```

The committed run itself used an absolute Go binary path because it was recorded
inside the Codex sandbox, but the failure shape is the same: one failed `go
test` run with Git state, a combined log, and replay instructions.

The snapshot shows:

- The failure: `TestIntentionalFailure` with `Multiply(3, 3) = 9, want 8`
- The log: `.capsule/capsules/cap_12a03ebbc337/logs/001-combined.log`
- The Git linkage: commit `973149dab65374825fba0838d0c22b7e808194e6` on `main`
- The replay command: `../../capsule replay cap_12a03ebbc337 --rerun`

## Inspect It

From this directory:

```sh
../../capsule summary cap_12a03ebbc337
../../capsule agent cap_12a03ebbc337
../../capsule replay cap_12a03ebbc337
```

Open the combined log directly:

`.capsule/capsules/cap_12a03ebbc337/logs/001-combined.log`

## Recreate It

If you want to record a fresh run instead of using the committed fixture:

```sh
../../capsule ci go test ./...
../../capsule summary --last --redact
../../capsule agent --last --redact
../../capsule bundle --last --redact
```
