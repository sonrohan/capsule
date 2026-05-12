---
name: capsule-release
description: Prepare and publish Capsule releases. Use when the user asks to bump Capsule's version, create a release commit, create or push a vX.Y.Z Git tag, run the GoReleaser release checks, or publish the macOS binary release workflow.
---

# Capsule Release

Use this skill from the Capsule repository root.

## Workflow

1. Inspect `git status --short --branch`. Do not start a release from a dirty tree unless the user explicitly asks to include the current changes.
2. Choose the next SemVer version. Accept `X.Y.Z` or `vX.Y.Z`, but tags must be `vX.Y.Z`.
3. Run `scripts/prepare_release.sh <version>` from this skill.
4. Review the resulting commit and tag.
5. Push only after the user has asked to publish:
   ```sh
   git push origin main
   git push origin vX.Y.Z
   ```

Pushing the tag triggers `.github/workflows/release.yml`, which runs GoReleaser and publishes the macOS artifact.

## Script

Use:

```sh
.agents/skills/capsule-release/scripts/prepare_release.sh v0.2.0
```

Useful options:

```sh
.agents/skills/capsule-release/scripts/prepare_release.sh --dry-run v0.2.0
.agents/skills/capsule-release/scripts/prepare_release.sh --push v0.2.0
```

The script:

- updates `VERSION`
- updates the fallback `version = "..."` in `main.go`
- runs `gofmt`, `go test ./...`, `sh -n install.sh`, and `go run github.com/goreleaser/goreleaser/v2@latest check`
- commits `Release vX.Y.Z`
- creates annotated tag `vX.Y.Z`
- pushes `main` and the tag only with `--push`

## Guardrails

- Do not create a duplicate tag. If the tag exists locally or remotely, stop and report it.
- Do not push without explicit user intent.
- If GoReleaser checking needs network access, request approval for the `go run github.com/goreleaser/goreleaser/v2@latest check` command.
- Keep release commits scoped to version metadata unless the user explicitly asks to include other changes.
