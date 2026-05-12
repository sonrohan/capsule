package main

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

func firstFailedCommand(session Session) *CommandRecord {
	for i := range session.Commands {
		if session.Commands[i].ExitCode != 0 {
			return &session.Commands[i]
		}
	}
	return nil
}

func summaryText(session Session, redact bool) string {
	text, _ := summaryTextWithConfig(session, redact, defaultConfig())
	return text
}

func summaryTextWithConfig(session Session, redact bool, config CapsuleConfig) (string, error) {
	if redact {
		redacted, err := redactSessionWithConfig(session, config)
		if err != nil {
			return "", err
		}
		session = redacted
	}
	var b strings.Builder
	failed := firstFailedCommand(session)
	fmt.Fprintf(&b, "# Capsule %s\n", session.ID)
	fmt.Fprintf(&b, "Git: %s on %s\n", fallback(session.Git.SHA, "unknown"), fallback(session.Git.Branch, "unknown"))
	fmt.Fprintf(&b, "Started: %s\n", session.StartedAt.Format(time.RFC3339))
	if session.FinishedAt != nil {
		fmt.Fprintf(&b, "Finished: %s\n", session.FinishedAt.Format(time.RFC3339))
	}
	fmt.Fprintf(&b, "Commands: %d\n", len(session.Commands))
	fmt.Fprintf(&b, "Artifacts: %d\n", len(session.Artifacts))
	if failed != nil {
		fmt.Fprintf(&b, "Failed: %s\n", failed.Command)
		fmt.Fprintf(&b, "Exit code: %d\n", failed.ExitCode)
		fmt.Fprintf(&b, "Log: %s\n", filepath.Join(capsuleSnapshotDir(session.ID), failed.Logs.Combined))
	} else {
		fmt.Fprintln(&b, "Failed: none")
	}
	fmt.Fprintf(&b, "Replay: capsule replay %s --rerun\n", session.ID)
	return b.String(), nil
}

func agentBriefing(session Session, redact bool) string {
	text, _ := agentBriefingWithConfig(session, redact, defaultConfig())
	return text
}

func agentBriefingWithConfig(session Session, redact bool, config CapsuleConfig) (string, error) {
	if redact {
		redacted, err := redactSessionWithConfig(session, config)
		if err != nil {
			return "", err
		}
		session = redacted
	}
	failed := firstFailedCommand(session)
	var b strings.Builder
	fmt.Fprintln(&b, "Debug this Capsule run.")
	fmt.Fprintln(&b)
	fmt.Fprintf(&b, "Capsule ID: %s\n", session.ID)
	fmt.Fprintf(&b, "Git SHA: %s\n", fallback(session.Git.SHA, "unknown"))
	fmt.Fprintf(&b, "Branch: %s\n", fallback(session.Git.Branch, "unknown"))
	if failed != nil {
		fmt.Fprintf(&b, "Failed command: %s\n", failed.Command)
		fmt.Fprintf(&b, "Exit code: %d\n", failed.ExitCode)
		fmt.Fprintf(&b, "Primary log: %s\n", filepath.Join(capsuleSnapshotDir(session.ID), failed.Logs.Combined))
	} else {
		fmt.Fprintln(&b, "Failed command: none")
	}
	if session.Git.Dirty {
		fmt.Fprintln(&b, "Working tree at capture: dirty")
	} else {
		fmt.Fprintln(&b, "Working tree at capture: clean")
	}
	fmt.Fprintln(&b, "Artifacts:")
	if len(session.Artifacts) == 0 {
		fmt.Fprintln(&b, "- none")
	} else {
		for _, artifact := range session.Artifacts {
			fmt.Fprintf(&b, "- %s (%s)\n", artifact.Path, artifact.Kind)
		}
	}
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "Start by reading manifest.json, commands.json, metadata.json, and the combined log before proposing a fix.")
	fmt.Fprintln(&b, "Do not infer the failure from prose alone; inspect the recorded evidence first.")
	return b.String(), nil
}
