package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func cmdStart() error {
	if _, err := os.Stat(activeSessionPath()); err == nil {
		return errors.New("an active session already exists; run 'capsule finish' first")
	}

	id, err := newID()
	if err != nil {
		return err
	}

	if err := ensureDirs(
		capsuleDir,
		filepath.Join(capsuleDir, "sessions", id, "logs"),
		filepath.Join(capsuleDir, "sessions", id, "artifacts"),
		filepath.Join(capsuleDir, "capsules"),
		filepath.Join(capsuleDir, "cache"),
	); err != nil {
		return err
	}

	env, err := collectEnvironment()
	if err != nil {
		return err
	}
	git, err := collectGitMetadata()
	if err != nil {
		return err
	}

	session := Session{
		ID:          id,
		StartedAt:   time.Now(),
		Git:         git,
		Environment: env,
		Commands:    []CommandRecord{},
		Artifacts:   []ArtifactRecord{},
		Metadata: map[string]string{
			"format_version": "1",
		},
	}
	if err := saveJSON(activeSessionPath(), session); err != nil {
		return err
	}
	if err := saveJSON(filepath.Join(sessionDir(id), "session.json"), session); err != nil {
		return err
	}

	fmt.Printf("Started Capsule %s\n", id)
	fmt.Printf("Git SHA: %s\n", fallback(git.SHA, "unknown"))
	fmt.Printf("Branch: %s\n", fallback(git.Branch, "unknown"))
	if git.Dirty {
		fmt.Println("Working tree: dirty")
	}
	return nil
}

func cmdRun(args []string) error {
	if len(args) == 0 {
		return errors.New("missing command")
	}
	session, err := loadActiveSession()
	if err != nil {
		return err
	}

	index := len(session.Commands) + 1
	logDir := filepath.Join(sessionDir(session.ID), "logs")
	if err := ensureDirs(logDir, filepath.Join(sessionDir(session.ID), "artifacts")); err != nil {
		return err
	}

	stdoutPath := filepath.Join(logDir, fmt.Sprintf("%03d-stdout.log", index))
	stderrPath := filepath.Join(logDir, fmt.Sprintf("%03d-stderr.log", index))
	combinedPath := filepath.Join(logDir, fmt.Sprintf("%03d-combined.log", index))

	stdoutFile, err := os.Create(stdoutPath)
	if err != nil {
		return err
	}
	defer stdoutFile.Close()
	stderrFile, err := os.Create(stderrPath)
	if err != nil {
		return err
	}
	defer stderrFile.Close()
	combinedFile, err := os.Create(combinedPath)
	if err != nil {
		return err
	}
	defer combinedFile.Close()

	start := time.Now()
	fmt.Printf("capsule run #%d: %s\n", index, strings.Join(args, " "))

	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdout = io.MultiWriter(os.Stdout, stdoutFile, combinedFile)
	cmd.Stderr = io.MultiWriter(os.Stderr, stderrFile, combinedFile)
	cmd.Stdin = os.Stdin

	runErr := cmd.Run()
	finished := time.Now()
	exitCode := 0
	if runErr != nil {
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = -1
		}
	}

	artifacts, detectErr := detectArtifacts(session.ID, index, start, args)
	if detectErr != nil {
		fmt.Fprintln(os.Stderr, "capsule: artifact detection failed:", detectErr)
	}

	record := CommandRecord{
		Index:      index,
		Args:       args,
		Command:    strings.Join(args, " "),
		StartedAt:  start,
		FinishedAt: finished,
		DurationMS: finished.Sub(start).Milliseconds(),
		ExitCode:   exitCode,
		Logs: CommandLogs{
			Stdout:   filepath.ToSlash(filepath.Join("logs", filepath.Base(stdoutPath))),
			Stderr:   filepath.ToSlash(filepath.Join("logs", filepath.Base(stderrPath))),
			Combined: filepath.ToSlash(filepath.Join("logs", filepath.Base(combinedPath))),
		},
		Artifacts: artifacts,
	}
	session.Commands = append(session.Commands, record)
	session.Artifacts = mergeArtifacts(session.Artifacts, artifacts)

	if err := persistActiveSession(session); err != nil {
		return err
	}
	fmt.Printf("Recorded command #%d: exit=%d duration=%dms artifacts=%d\n", index, exitCode, record.DurationMS, len(artifacts))
	if runErr != nil {
		if showRunFailureGuidance {
			fmt.Println()
			fmt.Println("Command failed.")
			fmt.Println()
			fmt.Println("Inspect:")
			fmt.Printf("  less %s\n", filepath.Join(sessionDir(session.ID), record.Logs.Combined))
			fmt.Println()
			fmt.Println("Finish snapshot:")
			fmt.Println("  capsule finish")
			fmt.Println()
			fmt.Println("Replay later:")
			fmt.Printf("  capsule replay %s --rerun\n", session.ID)
		}
		return &commandFailedError{code: exitCode}
	}
	return nil
}

func cmdFinish() error {
	session, err := loadActiveSession()
	if err != nil {
		return err
	}
	now := time.Now()
	session.FinishedAt = &now

	if err := persistActiveSession(session); err != nil {
		return err
	}

	dest := capsuleSnapshotDir(session.ID)
	if _, err := os.Stat(dest); err == nil {
		return fmt.Errorf("capsule snapshot %s already exists", session.ID)
	}
	if err := ensureDirs(dest); err != nil {
		return err
	}
	if err := copyDir(sessionDir(session.ID), dest); err != nil {
		return err
	}
	if err := writeSnapshotFiles(dest, session); err != nil {
		return err
	}
	if err := os.Remove(activeSessionPath()); err != nil {
		return err
	}

	fmt.Printf("Finished Capsule %s\n", session.ID)
	fmt.Printf("Snapshot: %s\n", dest)
	fmt.Printf("Commands: %d\n", len(session.Commands))
	fmt.Printf("Artifacts: %d\n", len(session.Artifacts))
	return nil
}

func cmdCI(args []string) error {
	if len(args) == 0 {
		return errors.New("missing command")
	}
	if _, err := os.Stat(activeSessionPath()); err == nil {
		return errors.New("an active session already exists; finish or remove it before running capsule ci")
	}

	if err := cmdStart(); err != nil {
		return err
	}
	session, err := loadActiveSession()
	if err != nil {
		return err
	}

	previousGuidance := showRunFailureGuidance
	showRunFailureGuidance = false
	runErr := cmdRun(args)
	showRunFailureGuidance = previousGuidance
	finishErr := cmdFinish()
	if finishErr != nil {
		return finishErr
	}

	fmt.Println()
	fmt.Println("Capsule CI snapshot ready.")
	if err := printSummary(session.ID, false); err != nil {
		return err
	}
	bundlePath, err := createBundle(session.ID, false)
	if err != nil {
		return err
	}
	fmt.Printf("Bundle: %s\n", bundlePath)

	if runErr != nil {
		var commandErr *commandFailedError
		if errors.As(runErr, &commandErr) {
			return &commandFailedError{code: commandErr.code, quiet: true}
		}
		return runErr
	}
	return nil
}

func cmdSummary(args []string) error {
	args, redact, err := parseRedactFlag(args)
	if err != nil {
		return err
	}
	id, err := capsuleIDFromArgs(args)
	if err != nil {
		return err
	}
	return printSummary(id, redact)
}

func cmdAgent(args []string) error {
	args, redact, err := parseRedactFlag(args)
	if err != nil {
		return err
	}
	id, err := capsuleIDFromArgs(args)
	if err != nil {
		return err
	}
	session, err := loadCapsule(id)
	if err != nil {
		return err
	}
	config, err := loadConfig()
	if err != nil {
		return err
	}
	text, err := agentBriefingWithConfig(session, redact, config)
	if err != nil {
		return err
	}
	fmt.Print(text)
	return nil
}

func cmdBundle(args []string) error {
	args, options, err := parseBundleOptions(args)
	if err != nil {
		return err
	}
	id, err := capsuleIDFromArgs(args)
	if err != nil {
		return err
	}
	path, err := createBundleWithOptions(id, options)
	if err != nil {
		return err
	}
	fmt.Printf("Bundle created: %s\n", path)
	return nil
}

func cmdImport(args []string) error {
	if len(args) == 0 {
		return errors.New("missing bundle path")
	}
	id, err := importBundle(args[0])
	if err != nil {
		return err
	}
	fmt.Printf("Imported Capsule %s\n", id)
	fmt.Printf("Snapshot: %s\n", capsuleSnapshotDir(id))
	return nil
}

func cmdReplay(args []string) error {
	if len(args) == 0 {
		return errors.New("missing capsule id")
	}
	id := args[0]
	rerun := false
	for _, arg := range args[1:] {
		if arg == "--rerun" {
			rerun = true
		}
	}

	session, err := loadCapsule(id)
	if err != nil {
		return err
	}
	currentGit, _ := collectGitMetadata()
	fmt.Printf("Restoring Capsule %s...\n", session.ID)
	fmt.Printf("Git SHA: %s\n", fallback(session.Git.SHA, "unknown"))
	fmt.Printf("Branch: %s\n", fallback(session.Git.Branch, "unknown"))
	if currentGit.SHA != "" && session.Git.SHA != "" && currentGit.SHA != session.Git.SHA {
		fmt.Printf("Current repo SHA differs: %s\n", shortSHA(currentGit.SHA))
		fmt.Printf("To inspect the exact source state, run: git checkout %s\n", session.Git.SHA)
	}
	fmt.Println("Commands:")
	for _, cmd := range session.Commands {
		fmt.Printf("  %d. %s [exit=%d, %dms]\n", cmd.Index, cmd.Command, cmd.ExitCode, cmd.DurationMS)
	}
	fmt.Println("Artifacts:")
	if len(session.Artifacts) == 0 {
		fmt.Println("  none")
	} else {
		for _, artifact := range session.Artifacts {
			fmt.Printf("  - %s (%s)\n", artifact.Path, artifact.Kind)
		}
	}
	fmt.Printf("Logs restored at: %s\n", filepath.Join(capsuleSnapshotDir(session.ID), "logs"))
	fmt.Println("Ready.")

	if rerun {
		fmt.Println()
		fmt.Println("Rerunning recorded commands...")
		for _, command := range session.Commands {
			fmt.Printf("capsule replay #%d: %s\n", command.Index, command.Command)
			cmd := exec.Command(command.Args[0], command.Args[1:]...)
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			cmd.Stdin = os.Stdin
			if err := cmd.Run(); err != nil {
				return fmt.Errorf("rerun command #%d failed: %w", command.Index, err)
			}
		}
	}
	return nil
}

func printSummary(id string, redact bool) error {
	session, err := loadCapsule(id)
	if err != nil {
		return err
	}
	config, err := loadConfig()
	if err != nil {
		return err
	}
	text, err := summaryTextWithConfig(session, redact, config)
	if err != nil {
		return err
	}
	fmt.Print(text)
	return nil
}

func cmdList() error {
	root := filepath.Join(capsuleDir, "capsules")
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		fmt.Println("No Capsules found.")
		return nil
	}
	if err != nil {
		return err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })

	found := false
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		session, err := loadCapsule(entry.Name())
		if err != nil {
			continue
		}
		found = true
		fmt.Printf("%s  %s  %s  commands=%d artifacts=%d\n",
			session.ID,
			shortSHA(session.Git.SHA),
			fallback(session.Git.Branch, "unknown"),
			len(session.Commands),
			len(session.Artifacts),
		)
	}
	if !found {
		fmt.Println("No Capsules found.")
	}
	return nil
}
