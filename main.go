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
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

const capsuleDir = ".capsule"

var (
	version = "0.1.3"
	commit  = ""
	date    = ""
)

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

type CapsuleView struct {
	ID               string
	GitSHA           string
	GitBranch        string
	CommandCount     int
	ArtifactCount    int
	StartedAt        string
	FailedCommand    *CommandRecord
	FailedLogPath    string
	FailedLogPreview string
	ReplayCommand    string
	BundlePath       string
	AgentBriefing    string
	Commands         []CommandRecord
	Artifacts        []ArtifactRecord
}

type Redactor struct {
	replacements []stringPair
	patterns     []redactionPattern
}

type stringPair struct {
	old string
	new string
}

type redactionPattern struct {
	pattern     *regexp.Regexp
	replacement string
}

type CapsuleConfig struct {
	Capture   CaptureConfig   `json:"capture"`
	Artifacts ArtifactConfig  `json:"artifacts"`
	Redaction RedactionConfig `json:"redaction"`
	Bundle    BundleConfig    `json:"bundle"`
}

type CaptureConfig struct {
	ScanRoots        []string `json:"scan_roots"`
	Include          []string `json:"include"`
	Exclude          []string `json:"exclude"`
	MaxArtifactBytes int64    `json:"max_artifact_bytes"`
}

type ArtifactConfig struct {
	Enabled        *bool               `json:"enabled"`
	Kinds          map[string][]string `json:"kinds"`
	Include        []string            `json:"include"`
	Exclude        []string            `json:"exclude"`
	CommandFilters map[string][]string `json:"command_filters"`
}

type RedactionConfig struct {
	Defaults *bool                  `json:"defaults"`
	Replace  []RedactionReplacement `json:"replace"`
	Literals []string               `json:"literals"`
	Allow    []string               `json:"allow"`
	Files    RedactionFilesConfig   `json:"files"`
}

type RedactionReplacement struct {
	Pattern string `json:"pattern"`
	With    string `json:"with"`
}

type RedactionFilesConfig struct {
	Include []string `json:"include"`
	Exclude []string `json:"exclude"`
}

type BundleConfig struct {
	Include []string `json:"include"`
	Exclude []string `json:"exclude"`
}

type BundleOptions struct {
	Redact  bool
	Include []string
	Exclude []string
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
	case "agent":
		err = cmdAgent(os.Args[2:])
	case "bundle":
		err = cmdBundle(os.Args[2:])
	case "import":
		err = cmdImport(os.Args[2:])
	case "list":
		err = cmdList()
	case "ui":
		err = cmdUI(os.Args[2:])
	case "version", "-v", "--version":
		cmdVersion()
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
  capsule summary <capsule-id|--last> [--redact]
  capsule agent <capsule-id|--last> [--redact]
  capsule bundle <capsule-id|--last> [--redact] [--include glob] [--exclude glob] [--no-artifacts]
  capsule import <bundle.zip>
  capsule list
  capsule ui [--port 3000]
  capsule version`)
}

func cmdVersion() {
	fmt.Println(versionString())
}

func versionString() string {
	parts := []string{"capsule", version}
	if commit != "" {
		parts = append(parts, commit)
	}
	if date != "" {
		parts = append(parts, date)
	}
	return strings.Join(parts, " ")
}

func defaultConfig() CapsuleConfig {
	enabled := true
	return CapsuleConfig{
		Capture: CaptureConfig{
			ScanRoots: []string{"."},
			Exclude:   []string{".git/**", ".capsule/**", "node_modules/**", ".gradle/**", "**/.git/**", "**/.capsule/**", "**/node_modules/**", "**/.gradle/**"},
		},
		Artifacts: ArtifactConfig{
			Enabled: &enabled,
			Kinds: map[string][]string{
				"android-apk":         {"**/*.apk"},
				"ios-ipa":             {"**/*.ipa"},
				"xcode-result":        {"**/*.xcresult"},
				"log":                 {"**/*.log"},
				"android-lint-report": {"**/lint-results*.html", "**/lint-results*.xml"},
				"junit-xml":           {"**/test-*.xml", "**/*junit*.xml", "**/surefire-reports/*.xml", "**/test-results/**/*.xml"},
				"screenshot":          {"**/*screenshot*.png", "**/*screenshot*.jpg", "**/*screenshot*.jpeg", "**/*snapshot*.png", "**/*snapshot*.jpg", "**/*snapshot*.jpeg", "**/*screenshot*/**/*.png", "**/*screenshot*/**/*.jpg", "**/*screenshot*/**/*.jpeg", "**/*snapshot*/**/*.png", "**/*snapshot*/**/*.jpg", "**/*snapshot*/**/*.jpeg"},
			},
			CommandFilters: map[string][]string{
				"gradle:lint":     {"android-lint-report", "log"},
				"gradle:test":     {"junit-xml", "log"},
				"gradle:assemble": {"android-apk", "ios-ipa", "log"},
				"gradle:bundle":   {"android-apk", "ios-ipa", "log"},
			},
		},
		Redaction: RedactionConfig{
			Defaults: &enabled,
			Files: RedactionFilesConfig{
				Include: []string{"logs/**", "manifest.json", "commands.json", "metadata.json", "session.json", "artifacts/**/*.json", "artifacts/**/*.log", "artifacts/**/*.txt", "artifacts/**/*.xml", "artifacts/**/*.html", "artifacts/**/*.md", "artifacts/**/*.yaml", "artifacts/**/*.yml", "artifacts/**/*.csv", "artifacts/**/*.sh"},
			},
		},
		Bundle: BundleConfig{
			Include: []string{"**"},
		},
	}
}

func loadConfig() (CapsuleConfig, error) {
	config := defaultConfig()
	if _, err := os.Stat("capsule.json"); os.IsNotExist(err) {
		return config, nil
	} else if err != nil {
		return config, err
	}

	var user CapsuleConfig
	if err := loadJSON("capsule.json", &user); err != nil {
		return config, err
	}
	mergeConfig(&config, user)
	return config, nil
}

func mergeConfig(base *CapsuleConfig, user CapsuleConfig) {
	if user.Capture.ScanRoots != nil {
		base.Capture.ScanRoots = user.Capture.ScanRoots
	}
	if user.Capture.Include != nil {
		base.Capture.Include = user.Capture.Include
	}
	if user.Capture.Exclude != nil {
		base.Capture.Exclude = append(base.Capture.Exclude, user.Capture.Exclude...)
	}
	if user.Capture.MaxArtifactBytes != 0 {
		base.Capture.MaxArtifactBytes = user.Capture.MaxArtifactBytes
	}
	if user.Artifacts.Kinds != nil {
		for kind, patterns := range user.Artifacts.Kinds {
			base.Artifacts.Kinds[kind] = patterns
		}
	}
	if user.Artifacts.Include != nil {
		base.Artifacts.Include = user.Artifacts.Include
	}
	if user.Artifacts.Exclude != nil {
		base.Artifacts.Exclude = append(base.Artifacts.Exclude, user.Artifacts.Exclude...)
	}
	if user.Artifacts.CommandFilters != nil {
		for filter, kinds := range user.Artifacts.CommandFilters {
			base.Artifacts.CommandFilters[filter] = kinds
		}
	}
	if user.Artifacts.Enabled != nil {
		base.Artifacts.Enabled = user.Artifacts.Enabled
	}
	if user.Redaction.Replace != nil {
		base.Redaction.Replace = append(base.Redaction.Replace, user.Redaction.Replace...)
	}
	if user.Redaction.Literals != nil {
		base.Redaction.Literals = append(base.Redaction.Literals, user.Redaction.Literals...)
	}
	if user.Redaction.Allow != nil {
		base.Redaction.Allow = append(base.Redaction.Allow, user.Redaction.Allow...)
	}
	if user.Redaction.Files.Include != nil {
		base.Redaction.Files.Include = user.Redaction.Files.Include
	}
	if user.Redaction.Files.Exclude != nil {
		base.Redaction.Files.Exclude = append(base.Redaction.Files.Exclude, user.Redaction.Files.Exclude...)
	}
	if user.Redaction.Defaults != nil {
		base.Redaction.Defaults = user.Redaction.Defaults
	}
	if user.Bundle.Include != nil {
		base.Bundle.Include = user.Bundle.Include
	}
	if user.Bundle.Exclude != nil {
		base.Bundle.Exclude = append(base.Bundle.Exclude, user.Bundle.Exclude...)
	}
}

func artifactsEnabled(config CapsuleConfig) bool {
	return config.Artifacts.Enabled == nil || *config.Artifacts.Enabled
}

func redactionDefaultsEnabled(config CapsuleConfig) bool {
	return config.Redaction.Defaults == nil || *config.Redaction.Defaults
}

func pathIncluded(path string, include, exclude []string) bool {
	path = filepath.ToSlash(filepath.Clean(path))
	if len(include) > 0 && !matchesAnyGlob(path, include) {
		return false
	}
	return !matchesAnyGlob(path, exclude)
}

func matchesAnyGlob(path string, patterns []string) bool {
	for _, pattern := range patterns {
		if matchGlob(pattern, path) {
			return true
		}
	}
	return false
}

func matchGlob(pattern, path string) bool {
	pattern = filepath.ToSlash(filepath.Clean(strings.TrimSpace(pattern)))
	path = filepath.ToSlash(filepath.Clean(path))
	if pattern == "." {
		return path == "."
	}
	if pattern == "**" {
		return true
	}
	if !strings.Contains(pattern, "/") {
		if ok, _ := filepath.Match(pattern, filepath.Base(path)); ok {
			return true
		}
	}
	regex, err := globRegex(pattern)
	if err != nil {
		return false
	}
	return regex.MatchString(path)
}

func globRegex(pattern string) (*regexp.Regexp, error) {
	var b strings.Builder
	b.WriteString("^")
	for i := 0; i < len(pattern); i++ {
		switch pattern[i] {
		case '*':
			if i+1 < len(pattern) && pattern[i+1] == '*' {
				i++
				if i+1 < len(pattern) && pattern[i+1] == '/' {
					b.WriteString("(?:.*/)?")
					i++
				} else {
					b.WriteString(".*")
				}
			} else {
				b.WriteString("[^/]*")
			}
		case '?':
			b.WriteString("[^/]")
		default:
			b.WriteString(regexp.QuoteMeta(string(pattern[i])))
		}
	}
	b.WriteString("$")
	return regexp.Compile(b.String())
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

func createBundle(id string, redact bool) (string, error) {
	return createBundleWithOptions(id, BundleOptions{Redact: redact})
}

func createBundleWithOptions(id string, options BundleOptions) (string, error) {
	config, err := loadConfig()
	if err != nil {
		return "", err
	}
	if len(options.Include) > 0 {
		config.Bundle.Include = options.Include
	}
	if len(options.Exclude) > 0 {
		config.Bundle.Exclude = append(config.Bundle.Exclude, options.Exclude...)
	}
	return createBundleWithConfig(id, options.Redact, config)
}

func createBundleWithConfig(id string, redact bool, config CapsuleConfig) (string, error) {
	src := capsuleSnapshotDir(id)
	if _, err := os.Stat(src); err != nil {
		return "", err
	}
	destDir := filepath.Join(capsuleDir, "bundles")
	if err := ensureDirs(destDir); err != nil {
		return "", err
	}
	name := id + ".zip"
	if redact {
		name = id + "-redacted.zip"
	}
	dest := filepath.Join(destDir, name)
	out, err := os.Create(dest)
	if err != nil {
		return "", err
	}
	defer out.Close()

	zw := zip.NewWriter(out)
	defer zw.Close()
	prefix := filepath.Join("capsule", id)
	session, err := loadCapsule(id)
	if err != nil {
		return "", err
	}
	var redactor Redactor
	if redact {
		redactor, err = newRedactorWithConfig(session, config)
		if err != nil {
			return "", err
		}
	}
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
		rel = filepath.ToSlash(rel)
		if !pathIncluded(rel, config.Bundle.Include, config.Bundle.Exclude) {
			return nil
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
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if redact && shouldRedactFileWithConfig(rel, config) {
			data = []byte(redactor.RedactText(string(data)))
		}
		_, err = writer.Write(data)
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
		views := make([]CapsuleView, 0, len(capsules))
		for _, capsule := range capsules {
			views = append(views, newCapsuleView(capsule))
		}
		if err := uiTemplate.Execute(w, views); err != nil {
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
		"capsule": versionString(),
		"go":      runtime.Version(),
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

func detectArtifacts(sessionID string, commandIndex int, since time.Time, args []string) ([]ArtifactRecord, error) {
	config, err := loadConfig()
	if err != nil {
		return nil, err
	}
	return detectArtifactsWithConfig(config, sessionID, commandIndex, since, args)
}

func detectArtifactsWithConfig(config CapsuleConfig, sessionID string, commandIndex int, since time.Time, args []string) ([]ArtifactRecord, error) {
	var artifacts []ArtifactRecord
	if !artifactsEnabled(config) {
		return artifacts, nil
	}
	roots := config.Capture.ScanRoots
	if len(roots) == 0 {
		roots = []string{"."}
	}
	destRoot := filepath.Join(sessionDir(sessionID), "artifacts")
	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			clean := filepath.Clean(path)
			if clean == "." {
				return nil
			}
			if d.IsDir() {
				if clean != "." && matchesAnyGlob(filepath.ToSlash(clean)+"/placeholder", config.Capture.Exclude) {
					return filepath.SkipDir
				}
				return nil
			}
			if !pathIncluded(clean, config.Capture.Include, config.Capture.Exclude) {
				return nil
			}
			if !pathIncluded(clean, config.Artifacts.Include, config.Artifacts.Exclude) {
				return nil
			}
			kind := artifactKindWithConfig(clean, config)
			if kind == "" {
				return nil
			}
			if !artifactMatchesCommandWithConfig(kind, args, config) {
				return nil
			}
			info, err := d.Info()
			if err != nil {
				return nil
			}
			if config.Capture.MaxArtifactBytes > 0 && info.Size() > config.Capture.MaxArtifactBytes {
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
		if err != nil {
			return nil, err
		}
	}
	sort.Slice(artifacts, func(i, j int) bool { return artifacts[i].Path < artifacts[j].Path })
	return artifacts, nil
}

func artifactMatchesCommand(kind string, args []string) bool {
	return artifactMatchesCommandWithConfig(kind, args, defaultConfig())
}

func artifactMatchesCommandWithConfig(kind string, args []string, config CapsuleConfig) bool {
	command := strings.ToLower(strings.Join(args, " "))
	filters := make([]string, 0, len(config.Artifacts.CommandFilters))
	for filter := range config.Artifacts.CommandFilters {
		filters = append(filters, filter)
	}
	sort.Slice(filters, func(i, j int) bool {
		iParts := strings.Count(filters[i], ":")
		jParts := strings.Count(filters[j], ":")
		if iParts != jParts {
			return iParts > jParts
		}
		return len(filters[i]) > len(filters[j])
	})
	for _, filter := range filters {
		kinds := config.Artifacts.CommandFilters[filter]
		parts := strings.Split(filter, ":")
		matches := true
		for _, part := range parts {
			if part != "" && !strings.Contains(command, strings.ToLower(part)) {
				matches = false
				break
			}
		}
		if matches {
			for _, allowed := range kinds {
				if kind == allowed {
					return true
				}
			}
			return false
		}
	}
	return true
}

func artifactKind(path string) string {
	return artifactKindWithConfig(path, defaultConfig())
}

func artifactKindWithConfig(path string, config CapsuleConfig) string {
	kinds := make([]string, 0, len(config.Artifacts.Kinds))
	for kind := range config.Artifacts.Kinds {
		kinds = append(kinds, kind)
	}
	sort.Strings(kinds)
	for _, kind := range kinds {
		if matchesAnyGlob(path, config.Artifacts.Kinds[kind]) {
			return kind
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

func parseRedactFlag(args []string) ([]string, bool, error) {
	var filtered []string
	redact := false
	for _, arg := range args {
		switch arg {
		case "--redact":
			redact = true
		default:
			if strings.HasPrefix(arg, "--") && arg != "--last" {
				return nil, false, fmt.Errorf("unknown flag %q", arg)
			}
			filtered = append(filtered, arg)
		}
	}
	return filtered, redact, nil
}

func parseBundleOptions(args []string) ([]string, BundleOptions, error) {
	var filtered []string
	options := BundleOptions{}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "--redact":
			options.Redact = true
		case "--include":
			if i+1 >= len(args) {
				return nil, options, errors.New("missing value for --include")
			}
			options.Include = append(options.Include, args[i+1])
			i++
		case "--exclude":
			if i+1 >= len(args) {
				return nil, options, errors.New("missing value for --exclude")
			}
			options.Exclude = append(options.Exclude, args[i+1])
			i++
		case "--no-artifacts":
			options.Exclude = append(options.Exclude, "artifacts/**")
		default:
			if strings.HasPrefix(arg, "--") && arg != "--last" {
				return nil, options, fmt.Errorf("unknown flag %q", arg)
			}
			filtered = append(filtered, arg)
		}
	}
	return filtered, options, nil
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

func redactSession(session Session) Session {
	redacted, _ := redactSessionWithConfig(session, defaultConfig())
	return redacted
}

func redactSessionWithConfig(session Session, config CapsuleConfig) (Session, error) {
	redactor, err := newRedactorWithConfig(session, config)
	if err != nil {
		return session, err
	}
	session.Git.Repository = redactor.RedactText(session.Git.Repository)
	session.Git.Status = redactor.RedactText(session.Git.Status)
	session.Environment.Hostname = redactor.RedactText(session.Environment.Hostname)
	session.Environment.User = redactor.RedactText(session.Environment.User)
	session.Environment.CWD = redactor.RedactText(session.Environment.CWD)
	session.Environment.Shell = redactor.RedactText(session.Environment.Shell)
	for key, value := range session.Environment.Runtime {
		session.Environment.Runtime[key] = redactor.RedactText(value)
	}
	for i := range session.Commands {
		session.Commands[i].Command = redactor.RedactText(session.Commands[i].Command)
		for j := range session.Commands[i].Args {
			session.Commands[i].Args[j] = redactor.RedactText(session.Commands[i].Args[j])
		}
		session.Commands[i].Logs.Stdout = redactor.RedactText(session.Commands[i].Logs.Stdout)
		session.Commands[i].Logs.Stderr = redactor.RedactText(session.Commands[i].Logs.Stderr)
		session.Commands[i].Logs.Combined = redactor.RedactText(session.Commands[i].Logs.Combined)
		for j := range session.Commands[i].Artifacts {
			session.Commands[i].Artifacts[j].Path = redactor.RedactText(session.Commands[i].Artifacts[j].Path)
			session.Commands[i].Artifacts[j].CapsulePath = redactor.RedactText(session.Commands[i].Artifacts[j].CapsulePath)
		}
	}
	for i := range session.Artifacts {
		session.Artifacts[i].Path = redactor.RedactText(session.Artifacts[i].Path)
		session.Artifacts[i].CapsulePath = redactor.RedactText(session.Artifacts[i].CapsulePath)
	}
	return session, nil
}

func newRedactor(session Session) Redactor {
	redactor, _ := newRedactorWithConfig(session, defaultConfig())
	return redactor
}

func newRedactorWithConfig(session Session, config CapsuleConfig) (Redactor, error) {
	var replacements []stringPair
	if redactionDefaultsEnabled(config) {
		for _, candidate := range []string{
			session.Environment.User,
			session.Environment.Hostname,
			session.Environment.CWD,
			session.Git.Repository,
			os.Getenv("HOME"),
		} {
			candidate = strings.TrimSpace(candidate)
			if candidate == "" {
				continue
			}
			replacements = append(replacements, stringPair{old: candidate, new: "[REDACTED]"})
		}
	}
	for _, candidate := range config.Redaction.Literals {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		replacements = append(replacements, stringPair{old: candidate, new: "[REDACTED]"})
	}
	for _, allowed := range config.Redaction.Allow {
		allowed = strings.TrimSpace(allowed)
		if allowed == "" {
			continue
		}
		filtered := replacements[:0]
		for _, replacement := range replacements {
			if replacement.old != allowed {
				filtered = append(filtered, replacement)
			}
		}
		replacements = filtered
	}
	sort.Slice(replacements, func(i, j int) bool {
		return len(replacements[i].old) > len(replacements[j].old)
	})
	patterns := []redactionPattern{}
	if redactionDefaultsEnabled(config) {
		patterns = append(patterns, redactionPattern{
			pattern:     regexp.MustCompile(`(?i)\b(?:ghp|gho|ghu|github_pat|sk|rk|pat)_[A-Za-z0-9_\-]{8,}\b`),
			replacement: "[REDACTED_TOKEN]",
		})
	}
	for _, replacement := range config.Redaction.Replace {
		compiled, err := regexp.Compile(replacement.Pattern)
		if err != nil {
			return Redactor{}, fmt.Errorf("invalid redaction pattern %q: %w", replacement.Pattern, err)
		}
		with := replacement.With
		if with == "" {
			with = "[REDACTED]"
		}
		patterns = append(patterns, redactionPattern{pattern: compiled, replacement: with})
	}
	return Redactor{replacements: replacements, patterns: patterns}, nil
}

func (r Redactor) RedactText(input string) string {
	output := input
	for _, replacement := range r.replacements {
		output = strings.ReplaceAll(output, replacement.old, replacement.new)
	}
	output = strings.ReplaceAll(output, "/Users/[REDACTED]", "[REDACTED]")
	output = strings.ReplaceAll(output, "/home/[REDACTED]", "[REDACTED]")
	for _, pattern := range r.patterns {
		output = pattern.pattern.ReplaceAllString(output, pattern.replacement)
	}
	return output
}

func shouldRedactFile(rel string) bool {
	return shouldRedactFileWithConfig(rel, defaultConfig())
}

func shouldRedactFileWithConfig(rel string, config CapsuleConfig) bool {
	return pathIncluded(rel, config.Redaction.Files.Include, config.Redaction.Files.Exclude)
}

func isTextPath(name string) bool {
	lower := strings.ToLower(name)
	for _, ext := range []string{".json", ".log", ".txt", ".xml", ".html", ".md", ".yaml", ".yml", ".csv", ".sh"} {
		if strings.HasSuffix(lower, ext) {
			return true
		}
	}
	return false
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

func newCapsuleView(session Session) CapsuleView {
	view := CapsuleView{
		ID:            session.ID,
		GitSHA:        shortSHA(session.Git.SHA),
		GitBranch:     fallback(session.Git.Branch, "unknown"),
		CommandCount:  len(session.Commands),
		ArtifactCount: len(session.Artifacts),
		StartedAt:     session.StartedAt.Format(time.RFC3339),
		ReplayCommand: fmt.Sprintf("capsule replay %s --rerun", session.ID),
		BundlePath:    bundleLink(session.ID),
		AgentBriefing: agentBriefing(session, false),
		Commands:      session.Commands,
		Artifacts:     session.Artifacts,
	}
	if failed := firstFailedCommand(session); failed != nil {
		view.FailedCommand = failed
		view.FailedLogPath = filepath.ToSlash(filepath.Join("capsules", session.ID, failed.Logs.Combined))
		view.FailedLogPreview = readLogPreview(filepath.Join(capsuleSnapshotDir(session.ID), failed.Logs.Combined))
	}
	return view
}

func bundleLink(id string) string {
	path := filepath.Join(capsuleDir, "bundles", id+".zip")
	if _, err := os.Stat(path); err == nil {
		return filepath.ToSlash(filepath.Join("bundles", id+".zip"))
	}
	return ""
}

func readLogPreview(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	const maxChars = 1400
	text := string(data)
	if len(text) > maxChars {
		text = text[:maxChars] + "\n..."
	}
	return strings.TrimSpace(text)
}

func importBundle(path string) (string, error) {
	reader, err := zip.OpenReader(path)
	if err != nil {
		return "", err
	}
	defer reader.Close()

	var id string
	for _, file := range reader.File {
		parts := strings.Split(filepath.ToSlash(file.Name), "/")
		if len(parts) >= 2 && parts[0] == "capsule" && parts[1] != "" {
			id = parts[1]
			break
		}
	}
	if id == "" {
		return "", errors.New("bundle does not contain capsule/<id>/ entries")
	}

	dest := capsuleSnapshotDir(id)
	if _, err := os.Stat(dest); err == nil {
		return "", fmt.Errorf("capsule snapshot %s already exists", id)
	}
	if err := ensureDirs(dest); err != nil {
		return "", err
	}
	for _, file := range reader.File {
		parts := strings.Split(filepath.ToSlash(file.Name), "/")
		if len(parts) < 3 || parts[0] != "capsule" || parts[1] != id {
			continue
		}
		rel := filepath.Clean(filepath.Join(parts[2:]...))
		if rel == "." || rel == "" || rel == string(filepath.Separator) || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." {
			return "", fmt.Errorf("bundle contains invalid path %q", file.Name)
		}
		target := filepath.Join(dest, rel)
		if file.FileInfo().IsDir() {
			if err := ensureDirs(target); err != nil {
				return "", err
			}
			continue
		}
		if err := ensureDirs(filepath.Dir(target)); err != nil {
			return "", err
		}
		rc, err := file.Open()
		if err != nil {
			return "", err
		}
		out, err := os.Create(target)
		if err != nil {
			rc.Close()
			return "", err
		}
		_, copyErr := io.Copy(out, rc)
		closeErr := out.Close()
		rc.Close()
		if copyErr != nil {
			return "", copyErr
		}
		if closeErr != nil {
			return "", closeErr
		}
	}
	return id, nil
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
    :root { color-scheme: light; --ink:#162019; --muted:#667069; --line:#d8ded8; --panel:#f7f8f4; --accent:#126a5a; --bad:#a83232; --ok:#1f7a45; --warnbg:#fff3f0; }
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
    pre, textarea { font:13px/1.5 ui-monospace, SFMono-Regular, Menlo, monospace; }
    .ok { color:var(--ok); font-weight:700; }
    .bad { color:var(--bad); font-weight:700; }
    .empty { padding:48px; text-align:center; color:var(--muted); border:1px dashed var(--line); border-radius:8px; background:#fff; }
    .failure { border:1px solid #f2cbc4; background:var(--warnbg); border-radius:8px; padding:14px; margin-bottom:16px; }
    .preview { margin:10px 0 0; padding:12px; background:#fff; border:1px solid var(--line); border-radius:8px; overflow:auto; white-space:pre-wrap; }
    .tools { display:flex; flex-wrap:wrap; gap:10px; margin-top:12px; }
    .btn { border:1px solid var(--line); border-radius:8px; background:#fff; color:var(--ink); padding:8px 10px; cursor:pointer; }
    textarea { width:100%; min-height:220px; border:1px solid var(--line); border-radius:8px; padding:12px; resize:vertical; background:#fbfbf8; color:var(--ink); }
    .section-head { display:flex; justify-content:space-between; align-items:center; gap:12px; margin-bottom:10px; }
    a { color:var(--accent); }
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
            <span>{{.GitSHA}}</span>
            <span>{{.GitBranch}}</span>
            <span>{{.CommandCount}} commands</span>
            <span>{{.ArtifactCount}} artifacts</span>
            <span>{{.StartedAt}}</span>
          </div>
        </div>
        <code>{{.ReplayCommand}}</code>
      </div>
      <div class="section">
        {{if .FailedCommand}}
        <div class="failure">
          <div><strong>Failure</strong>: <code>{{.FailedCommand.Command}}</code> exited with <span class="bad">{{.FailedCommand.ExitCode}}</span>.</div>
          <div class="tools">
            <a class="btn" href="/files/{{.FailedLogPath}}">Open combined log</a>
            {{if .BundlePath}}<a class="btn" href="/files/{{.BundlePath}}">Download bundle</a>{{end}}
          </div>
          {{if .FailedLogPreview}}<pre class="preview">{{.FailedLogPreview}}</pre>{{end}}
        </div>
        {{end}}
        <div class="section-head">
          <h2>Agent Briefing</h2>
          <button class="btn" type="button" onclick="navigator.clipboard.writeText(document.getElementById('agent-{{.ID}}').value)">Copy</button>
        </div>
        <textarea id="agent-{{.ID}}" readonly>{{.AgentBriefing}}</textarea>
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
