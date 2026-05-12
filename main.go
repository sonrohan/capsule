package main

import (
	"archive/zip"
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

const capsuleDir = ".capsule"

var showRunFailureGuidance = true

type Session struct {
	ID          string            `json:"id"`
	StartedAt   time.Time         `json:"started_at"`
	FinishedAt  *time.Time        `json:"finished_at,omitempty"`
	Git         GitMetadata       `json:"git"`
	Environment Environment       `json:"environment"`
	Commands    []CommandRecord   `json:"commands"`
	Artifacts   []ArtifactRecord  `json:"artifacts"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

type GitMetadata struct {
	SHA        string `json:"sha"`
	Branch     string `json:"branch"`
	Dirty      bool   `json:"dirty"`
	Status     string `json:"status,omitempty"`
	Repository string `json:"repository"`
}

type Environment struct {
	OS       string            `json:"os"`
	Arch     string            `json:"arch"`
	Hostname string            `json:"hostname"`
	User     string            `json:"user"`
	CWD      string            `json:"cwd"`
	Shell    string            `json:"shell,omitempty"`
	Runtime  map[string]string `json:"runtime"`
}

type CommandRecord struct {
	Index      int              `json:"index"`
	Args       []string         `json:"args"`
	Command    string           `json:"command"`
	StartedAt  time.Time        `json:"started_at"`
	FinishedAt time.Time        `json:"finished_at"`
	DurationMS int64            `json:"duration_ms"`
	ExitCode   int              `json:"exit_code"`
	Logs       CommandLogs      `json:"logs"`
	Artifacts  []ArtifactRecord `json:"artifacts,omitempty"`
}

type CommandLogs struct {
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	Combined string `json:"combined"`
}

type ArtifactRecord struct {
	Path         string    `json:"path"`
	CapsulePath  string    `json:"capsule_path"`
	Kind         string    `json:"kind"`
	SizeBytes    int64     `json:"size_bytes"`
	DetectedAt   time.Time `json:"detected_at"`
	CommandIndex int       `json:"command_index,omitempty"`
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	var err error
	switch os.Args[1] {
	case "start":
		err = cmdStart()
	case "run":
		err = cmdRun(os.Args[2:])
	case "finish":
		err = cmdFinish()
	case "replay":
		err = cmdReplay(os.Args[2:])
	case "ci":
		err = cmdCI(os.Args[2:])
	case "summary":
		err = cmdSummary(os.Args[2:])
	case "bundle":
		err = cmdBundle(os.Args[2:])
	case "list":
		err = cmdList()
	case "ui":
		err = cmdUI(os.Args[2:])
	case "help", "-h", "--help":
		usage()
	default:
		err = fmt.Errorf("unknown command %q", os.Args[1])
	}

	if err != nil {
		var commandErr *commandFailedError
		if errors.As(err, &commandErr) {
			if !commandErr.quiet {
				fmt.Fprintln(os.Stderr, "capsule:", commandErr.Error())
			}
			code := commandErr.code
			if code < 1 {
				code = 1
			}
			os.Exit(code)
		}
		fmt.Fprintln(os.Stderr, "capsule:", err)
		os.Exit(1)
	}
}

type commandFailedError struct {
	code  int
	quiet bool
}

func (e *commandFailedError) Error() string {
	return fmt.Sprintf("command exited with code %d", e.code)
}

func usage() {
	fmt.Println(`Capsule tracks replayable execution sessions for Git repos.

Usage:
  capsule start
  capsule run <command> [args...]
  capsule finish
  capsule replay <capsule-id> [--rerun]
  capsule ci <command> [args...]
  capsule summary <capsule-id|--last>
  capsule bundle <capsule-id|--last>
  capsule list
  capsule ui [--port 3000]`)
}

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

	artifacts, detectErr := detectArtifacts(session.ID, index, start)
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
	if err := printSummary(session.ID); err != nil {
		return err
	}
	bundlePath, err := createBundle(session.ID)
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
	id, err := capsuleIDFromArgs(args)
	if err != nil {
		return err
	}
	return printSummary(id)
}

func cmdBundle(args []string) error {
	id, err := capsuleIDFromArgs(args)
	if err != nil {
		return err
	}
	path, err := createBundle(id)
	if err != nil {
		return err
	}
	fmt.Printf("Bundle created: %s\n", path)
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

func printSummary(id string) error {
	session, err := loadCapsule(id)
	if err != nil {
		return err
	}
	failed := firstFailedCommand(session)
	fmt.Printf("# Capsule %s\n", session.ID)
	fmt.Printf("Git: %s on %s\n", fallback(session.Git.SHA, "unknown"), fallback(session.Git.Branch, "unknown"))
	fmt.Printf("Started: %s\n", session.StartedAt.Format(time.RFC3339))
	if session.FinishedAt != nil {
		fmt.Printf("Finished: %s\n", session.FinishedAt.Format(time.RFC3339))
	}
	fmt.Printf("Commands: %d\n", len(session.Commands))
	fmt.Printf("Artifacts: %d\n", len(session.Artifacts))
	if failed != nil {
		fmt.Printf("Failed: %s\n", failed.Command)
		fmt.Printf("Exit code: %d\n", failed.ExitCode)
		fmt.Printf("Log: %s\n", filepath.Join(capsuleSnapshotDir(session.ID), failed.Logs.Combined))
	} else {
		fmt.Println("Failed: none")
	}
	fmt.Printf("Replay: capsule replay %s --rerun\n", session.ID)
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

func createBundle(id string) (string, error) {
	src := capsuleSnapshotDir(id)
	if _, err := os.Stat(src); err != nil {
		return "", err
	}
	destDir := filepath.Join(capsuleDir, "bundles")
	if err := ensureDirs(destDir); err != nil {
		return "", err
	}
	dest := filepath.Join(destDir, id+".zip")
	out, err := os.Create(dest)
	if err != nil {
		return "", err
	}
	defer out.Close()

	zw := zip.NewWriter(out)
	defer zw.Close()
	prefix := filepath.Join("capsule", id)
	err = filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		header.Name = filepath.ToSlash(filepath.Join(prefix, rel))
		header.Method = zip.Deflate
		writer, err := zw.CreateHeader(header)
		if err != nil {
			return err
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		_, err = io.Copy(writer, in)
		return err
	})
	if err != nil {
		return "", err
	}
	return dest, nil
}

func cmdUI(args []string) error {
	port := 3000
	for i := 0; i < len(args); i++ {
		if args[i] == "--port" && i+1 < len(args) {
			parsed, err := strconv.Atoi(args[i+1])
			if err != nil {
				return err
			}
			port = parsed
			i++
		}
	}

	mux := http.NewServeMux()
	mux.Handle("/files/", http.StripPrefix("/files/", http.FileServer(http.Dir(capsuleDir))))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		capsules, err := allCapsules()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := uiTemplate.Execute(w, capsules); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	fmt.Printf("Capsule UI: http://%s\n", addr)
	return http.ListenAndServe(addr, mux)
}

func collectGitMetadata() (GitMetadata, error) {
	root := runGit("rev-parse", "--show-toplevel")
	sha := runGit("rev-parse", "HEAD")
	branch := runGit("branch", "--show-current")
	status := runGit("status", "--short")
	return GitMetadata{
		SHA:        strings.TrimSpace(sha),
		Branch:     strings.TrimSpace(branch),
		Dirty:      strings.TrimSpace(status) != "",
		Status:     strings.TrimSpace(status),
		Repository: strings.TrimSpace(root),
	}, nil
}

func collectEnvironment() (Environment, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return Environment{}, err
	}
	host, _ := os.Hostname()
	runtimeVersions := map[string]string{
		"go": runtime.Version(),
	}
	for _, candidate := range []string{"node", "npm", "java", "javac", "gradle", "docker", "python3", "ruby"} {
		if version := toolVersion(candidate); version != "" {
			runtimeVersions[candidate] = version
		}
	}
	return Environment{
		OS:       runtime.GOOS,
		Arch:     runtime.GOARCH,
		Hostname: host,
		User:     os.Getenv("USER"),
		CWD:      cwd,
		Shell:    os.Getenv("SHELL"),
		Runtime:  runtimeVersions,
	}, nil
}

func toolVersion(name string) string {
	if _, err := exec.LookPath(name); err != nil {
		return ""
	}
	args := []string{"--version"}
	if name == "java" || name == "javac" {
		args = []string{"-version"}
	}
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil && len(out) == 0 {
		return ""
	}
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	if scanner.Scan() {
		return strings.TrimSpace(scanner.Text())
	}
	return strings.TrimSpace(string(out))
}

func detectArtifacts(sessionID string, commandIndex int, since time.Time) ([]ArtifactRecord, error) {
	var artifacts []ArtifactRecord
	destRoot := filepath.Join(sessionDir(sessionID), "artifacts")
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		clean := filepath.Clean(path)
		if clean == "." {
			return nil
		}
		if d.IsDir() {
			base := filepath.Base(clean)
			if base == ".git" || base == ".capsule" || base == "node_modules" || base == ".gradle" {
				return filepath.SkipDir
			}
			return nil
		}
		kind := artifactKind(clean)
		if kind == "" {
			return nil
		}
		info, err := d.Info()
		if err != nil || info.ModTime().Before(since.Add(-time.Second)) {
			return nil
		}
		artifactName := fmt.Sprintf("%03d-%s", commandIndex, strings.ReplaceAll(filepath.ToSlash(clean), "/", "__"))
		capsulePath := filepath.Join(destRoot, artifactName)
		if err := copyFile(clean, capsulePath); err != nil {
			return nil
		}
		artifacts = append(artifacts, ArtifactRecord{
			Path:         filepath.ToSlash(clean),
			CapsulePath:  filepath.ToSlash(filepath.Join("artifacts", artifactName)),
			Kind:         kind,
			SizeBytes:    info.Size(),
			DetectedAt:   time.Now(),
			CommandIndex: commandIndex,
		})
		return nil
	})
	sort.Slice(artifacts, func(i, j int) bool { return artifacts[i].Path < artifacts[j].Path })
	return artifacts, err
}

func artifactKind(path string) string {
	lower := strings.ToLower(filepath.Base(path))
	full := strings.ToLower(filepath.ToSlash(path))
	switch {
	case strings.HasSuffix(lower, ".apk"):
		return "android-apk"
	case strings.HasSuffix(lower, ".ipa"):
		return "ios-ipa"
	case strings.HasSuffix(lower, ".xcresult"):
		return "xcode-result"
	case strings.HasSuffix(lower, ".log"):
		return "log"
	case strings.HasSuffix(lower, ".png") || strings.HasSuffix(lower, ".jpg") || strings.HasSuffix(lower, ".jpeg"):
		if strings.Contains(full, "screenshot") || strings.Contains(full, "snapshot") {
			return "screenshot"
		}
	case strings.HasSuffix(lower, ".xml"):
		if strings.HasPrefix(lower, "test-") || strings.Contains(lower, "junit") || strings.Contains(full, "surefire-reports") || strings.Contains(full, "test-results") {
			return "junit-xml"
		}
	}
	return ""
}

func loadActiveSession() (Session, error) {
	var session Session
	if err := loadJSON(activeSessionPath(), &session); err != nil {
		if os.IsNotExist(err) {
			return session, errors.New("no active session; run 'capsule start' first")
		}
		return session, err
	}
	return session, nil
}

func persistActiveSession(session Session) error {
	if err := saveJSON(activeSessionPath(), session); err != nil {
		return err
	}
	return saveJSON(filepath.Join(sessionDir(session.ID), "session.json"), session)
}

func loadCapsule(id string) (Session, error) {
	var session Session
	path := filepath.Join(capsuleSnapshotDir(id), "manifest.json")
	if err := loadJSON(path, &session); err != nil {
		return session, err
	}
	return session, nil
}

func allCapsules() ([]Session, error) {
	root := filepath.Join(capsuleDir, "capsules")
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var capsules []Session
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		session, err := loadCapsule(entry.Name())
		if err == nil {
			capsules = append(capsules, session)
		}
	}
	sort.Slice(capsules, func(i, j int) bool { return capsules[i].StartedAt.After(capsules[j].StartedAt) })
	return capsules, nil
}

func capsuleIDFromArgs(args []string) (string, error) {
	if len(args) == 0 || args[0] == "--last" {
		return lastCapsuleID()
	}
	return args[0], nil
}

func lastCapsuleID() (string, error) {
	capsules, err := allCapsules()
	if err != nil {
		return "", err
	}
	if len(capsules) == 0 {
		return "", errors.New("no finished Capsules found")
	}
	return capsules[0].ID, nil
}

func firstFailedCommand(session Session) *CommandRecord {
	for i := range session.Commands {
		if session.Commands[i].ExitCode != 0 {
			return &session.Commands[i]
		}
	}
	return nil
}

func writeSnapshotFiles(dest string, session Session) error {
	if err := saveJSON(filepath.Join(dest, "manifest.json"), session); err != nil {
		return err
	}
	if err := saveJSON(filepath.Join(dest, "commands.json"), session.Commands); err != nil {
		return err
	}
	metadata := map[string]any{
		"id":          session.ID,
		"started_at":  session.StartedAt,
		"finished_at": session.FinishedAt,
		"git":         session.Git,
		"environment": session.Environment,
		"artifacts":   session.Artifacts,
	}
	return saveJSON(filepath.Join(dest, "metadata.json"), metadata)
}

func runGit(args ...string) string {
	cmd := exec.Command("git", args...)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return string(out)
}

func saveJSON(path string, value any) error {
	if err := ensureDirs(filepath.Dir(path)); err != nil {
		return err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func loadJSON(path string, value any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, value)
}

func copyDir(src, dest string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dest, rel)
		if d.IsDir() {
			return ensureDirs(target)
		}
		return copyFile(path, target)
	})
}

func copyFile(src, dest string) error {
	if err := ensureDirs(filepath.Dir(dest)); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func ensureDirs(paths ...string) error {
	for _, path := range paths {
		if err := os.MkdirAll(path, 0o755); err != nil {
			return err
		}
	}
	return nil
}

func activeSessionPath() string {
	return filepath.Join(capsuleDir, "session.json")
}

func sessionDir(id string) string {
	return filepath.Join(capsuleDir, "sessions", id)
}

func capsuleSnapshotDir(id string) string {
	return filepath.Join(capsuleDir, "capsules", id)
}

func newID() (string, error) {
	var b [6]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return "cap_" + hex.EncodeToString(b[:]), nil
}

func fallback(value, replacement string) string {
	if strings.TrimSpace(value) == "" {
		return replacement
	}
	return value
}

func shortSHA(sha string) string {
	if len(sha) >= 7 {
		return sha[:7]
	}
	return fallback(sha, "unknown")
}

func mergeArtifacts(existing, next []ArtifactRecord) []ArtifactRecord {
	seen := map[string]bool{}
	var merged []ArtifactRecord
	for _, artifact := range append(existing, next...) {
		key := artifact.CapsulePath
		if seen[key] {
			continue
		}
		seen[key] = true
		merged = append(merged, artifact)
	}
	return merged
}

var uiTemplate = template.Must(template.New("ui").Funcs(template.FuncMap{
	"short": shortSHA,
}).Parse(`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Capsule</title>
  <style>
    :root { color-scheme: light; --ink:#162019; --muted:#667069; --line:#d8ded8; --panel:#f7f8f4; --accent:#126a5a; --bad:#a83232; --ok:#1f7a45; }
    * { box-sizing: border-box; }
    body { margin:0; font:14px/1.45 ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; color:var(--ink); background:#fdfdf9; }
    header { padding:28px 32px 20px; border-bottom:1px solid var(--line); background:#fff; }
    h1 { margin:0; font-size:28px; letter-spacing:0; }
    .sub { margin-top:6px; color:var(--muted); }
    main { max-width:1120px; margin:0 auto; padding:24px; }
    .capsule { border:1px solid var(--line); border-radius:8px; background:#fff; margin-bottom:18px; overflow:hidden; }
    .cap-head { display:flex; gap:16px; justify-content:space-between; padding:16px 18px; background:var(--panel); border-bottom:1px solid var(--line); }
    .id { font-family:ui-monospace, SFMono-Regular, Menlo, monospace; font-weight:700; color:var(--accent); }
    .meta { display:flex; flex-wrap:wrap; gap:10px 18px; color:var(--muted); }
    .section { padding:16px 18px; }
    h2 { font-size:15px; margin:0 0 10px; }
    table { width:100%; border-collapse:collapse; }
    th, td { text-align:left; padding:9px 8px; border-top:1px solid var(--line); vertical-align:top; }
    th { color:var(--muted); font-weight:600; font-size:12px; text-transform:uppercase; }
    code { font-family:ui-monospace, SFMono-Regular, Menlo, monospace; font-size:13px; }
    .ok { color:var(--ok); font-weight:700; }
    .bad { color:var(--bad); font-weight:700; }
    .empty { padding:48px; text-align:center; color:var(--muted); border:1px dashed var(--line); border-radius:8px; background:#fff; }
  </style>
</head>
<body>
  <header>
    <h1>Capsule</h1>
    <div class="sub">Replayable execution sessions attached to Git commits.</div>
  </header>
  <main>
    {{if not .}}<div class="empty">No finished Capsules yet. Run <code>capsule start</code>, <code>capsule run</code>, then <code>capsule finish</code>.</div>{{end}}
    {{range .}}
    {{$cap := .}}
    <article class="capsule">
      <div class="cap-head">
        <div>
          <div class="id">{{.ID}}</div>
          <div class="meta">
            <span>{{short .Git.SHA}}</span>
            <span>{{.Git.Branch}}</span>
            <span>{{len .Commands}} commands</span>
            <span>{{len .Artifacts}} artifacts</span>
          </div>
        </div>
        <code>capsule replay {{.ID}}</code>
      </div>
      <div class="section">
        <h2>Execution Timeline</h2>
        <table>
          <thead><tr><th>#</th><th>Command</th><th>Exit</th><th>Duration</th><th>Logs</th></tr></thead>
          <tbody>
            {{range .Commands}}
            <tr>
              <td>{{.Index}}</td>
              <td><code>{{.Command}}</code></td>
              <td>{{if eq .ExitCode 0}}<span class="ok">0</span>{{else}}<span class="bad">{{.ExitCode}}</span>{{end}}</td>
              <td>{{.DurationMS}}ms</td>
              <td><a href="/files/capsules/{{$cap.ID}}/{{.Logs.Combined}}"><code>{{.Logs.Combined}}</code></a></td>
            </tr>
            {{end}}
          </tbody>
        </table>
      </div>
      <div class="section">
        <h2>Artifacts</h2>
        {{if not .Artifacts}}<div class="meta">No artifacts detected.</div>{{end}}
        {{if .Artifacts}}
        <table>
          <thead><tr><th>Kind</th><th>Path</th><th>Size</th></tr></thead>
          <tbody>
            {{range .Artifacts}}<tr><td>{{.Kind}}</td><td><a href="/files/capsules/{{$cap.ID}}/{{.CapsulePath}}"><code>{{.Path}}</code></a></td><td>{{.SizeBytes}} bytes</td></tr>{{end}}
          </tbody>
        </table>
        {{end}}
      </div>
    </article>
    {{end}}
  </main>
</body>
</html>`))
