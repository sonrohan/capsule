# Agent Debugging Handoff

Capsule is useful for AI agents because it turns a failure into a structured
debugging package.

When a run fails:

```sh
capsule ci go test ./...
capsule agent --last --redact
capsule bundle --last --redact
```

`capsule agent --last --redact` produces a ready-to-paste brief that tells the
agent to start from recorded evidence instead of from prose:

```text
Debug this Capsule run.

Capsule ID: cap_x
Git SHA: ...
Branch: ...
Failed command: go test ./...
Exit code: 1
Primary log: .capsule/capsules/cap_x/logs/001-combined.log
Artifacts:
- ...
```

The bundle contains:

```text
manifest.json
commands.json
metadata.json
logs/
artifacts/
```

That lets the receiver answer concrete questions before changing code:

- What command failed?
- What exit code did it return?
- What Git SHA was tested?
- Was the working tree dirty?
- Which logs and artifacts matter?
