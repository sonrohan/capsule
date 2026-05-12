# Local Failure Repro

Use Capsule when a command fails and you want to share the full repro context.

```sh
./capsule ci go test ./...
./capsule summary --last
./capsule bundle --last
```

Send the summary and `.capsule/bundles/<id>.zip` to another developer.

They can inspect:

```sh
./capsule replay <id>
```

Or rerun:

```sh
./capsule replay <id> --rerun
```

The value is not that Capsule magically fixes the issue. The value is that the
debugging handoff starts from the exact command, logs, artifacts, Git SHA, and
environment metadata instead of a vague description.
