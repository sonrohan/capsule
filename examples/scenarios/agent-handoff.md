# Agent Debugging Handoff

Capsule is useful for AI agents because it turns a failed run into structured
inputs.

Instead of this:

```text
Tests failed, here is some terminal output...
```

Give the agent a Capsule bundle containing:

```text
manifest.json
commands.json
metadata.json
logs/001-combined.log
artifacts/
```

The agent can answer concrete questions:

- What command failed?
- What exit code did it return?
- What Git SHA was tested?
- Was the working tree dirty?
- What runtime versions were present?
- Which logs and artifacts are relevant?

This makes Capsule useful even before adding any AI-specific features.
