package main

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestArtifactKind(t *testing.T) {
	cases := map[string]string{
		"app/build/outputs/apk/debug/app-debug.apk":       "android-apk",
		"build/test-results/test/TEST-AppTest.xml":        "junit-xml",
		"target/surefire-reports/junit-results.xml":       "junit-xml",
		"reports/screenshots/login-failure.png":           "screenshot",
		"logs/build.log":                                  "log",
		"ios/build/app.ipa":                               "ios-ipa",
		"app/build/reports/lint-results-debug.html":       "android-lint-report",
		"app/build/reports/lint-results-debug.xml":        "android-lint-report",
		"composeApp/src/main/res/drawable/background.png": "",
	}

	for path, want := range cases {
		if got := artifactKind(path); got != want {
			t.Fatalf("artifactKind(%q) = %q, want %q", path, got, want)
		}
	}
}

func TestShortSHA(t *testing.T) {
	if got := shortSHA("abcdef123456"); got != "abcdef1" {
		t.Fatalf("shortSHA returned %q", got)
	}
	if got := shortSHA(""); got != "unknown" {
		t.Fatalf("shortSHA empty returned %q", got)
	}
}

func TestVersionString(t *testing.T) {
	oldVersion, oldCommit, oldDate := version, commit, date
	t.Cleanup(func() {
		version, commit, date = oldVersion, oldCommit, oldDate
	})

	version, commit, date = "1.2.3", "abc1234", "2026-05-11T12:34:56Z"
	if got, want := versionString(), "capsule 1.2.3 abc1234 2026-05-11T12:34:56Z"; got != want {
		t.Fatalf("versionString() = %q, want %q", got, want)
	}

	version, commit, date = "1.2.3", "", ""
	if got, want := versionString(), "capsule 1.2.3"; got != want {
		t.Fatalf("versionString() without build metadata = %q, want %q", got, want)
	}
}

func TestDefaultVersionMatchesVersionFile(t *testing.T) {
	data, err := os.ReadFile("VERSION")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := version, strings.TrimSpace(string(data)); got != want {
		t.Fatalf("default version = %q, VERSION file = %q", got, want)
	}
}

func TestArtifactMatchesGradleCommand(t *testing.T) {
	if !artifactMatchesCommand("android-apk", []string{"./gradlew", ":app:assembleDebug"}) {
		t.Fatal("assemble should match APK artifacts")
	}
	if artifactMatchesCommand("junit-xml", []string{"./gradlew", ":app:assembleDebug"}) {
		t.Fatal("assemble should not match JUnit artifacts")
	}
	if !artifactMatchesCommand("junit-xml", []string{"./gradlew", ":app:testDebugUnitTest"}) {
		t.Fatal("test should match JUnit artifacts")
	}
	if !artifactMatchesCommand("android-lint-report", []string{"./gradlew", ":app:lintDebug"}) {
		t.Fatal("lint should match Android lint reports")
	}
}

func TestFirstFailedCommand(t *testing.T) {
	session := Session{
		Commands: []CommandRecord{
			{Index: 1, Command: "go test ./...", ExitCode: 0},
			{Index: 2, Command: "go test ./pkg", ExitCode: 1},
		},
	}

	failed := firstFailedCommand(session)
	if failed == nil {
		t.Fatal("expected failed command")
	}
	if failed.Index != 2 {
		t.Fatalf("failed command index = %d, want 2", failed.Index)
	}
}

func TestAgentBriefingIncludesFailedCommand(t *testing.T) {
	session := Session{
		ID: "cap_demo",
		Git: GitMetadata{
			SHA:    "abcdef1234567890",
			Branch: "main",
		},
		Commands: []CommandRecord{
			{Index: 1, Command: "go test ./...", ExitCode: 1, Logs: CommandLogs{Combined: "logs/001-combined.log"}},
		},
		Artifacts: []ArtifactRecord{
			{Path: "reports/junit.xml", Kind: "junit-xml"},
		},
	}

	text := agentBriefing(session, false)
	for _, want := range []string{
		"Debug this Capsule run.",
		"Failed command: go test ./...",
		"Exit code: 1",
		"Primary log: .capsule/capsules/cap_demo/logs/001-combined.log",
		"- reports/junit.xml (junit-xml)",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("agent briefing missing %q\n%s", want, text)
		}
	}
}

func TestCreateBundleRedactsSensitiveContent(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	tmp := t.TempDir()
	if err := os.Chdir(tmp); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(wd)
	})
	t.Setenv("HOME", "/Users/alice")

	finished := time.Now()
	session := Session{
		ID:         "cap_redact",
		StartedAt:  finished.Add(-time.Minute),
		FinishedAt: &finished,
		Git: GitMetadata{
			SHA:        "abcdef1234567890",
			Branch:     "main",
			Repository: "/Users/alice/repos/demo",
		},
		Environment: Environment{
			Hostname: "Alice-MBP.local",
			User:     "alice",
			CWD:      "/Users/alice/repos/demo",
			Shell:    "/bin/zsh",
			Runtime: map[string]string{
				"node": "v22.0.0",
			},
		},
		Commands: []CommandRecord{
			{
				Index:    1,
				Args:     []string{"go", "test", "./..."},
				Command:  "API_KEY=sk_abcd1234567890 go test ./...",
				ExitCode: 1,
				Logs: CommandLogs{
					Combined: "logs/001-combined.log",
				},
			},
		},
		Artifacts: []ArtifactRecord{
			{
				Path:        "reports/failure.txt",
				CapsulePath: "artifacts/001-failure.txt",
				Kind:        "log",
			},
		},
	}
	snapshot := capsuleSnapshotDir(session.ID)
	if err := ensureDirs(filepath.Join(snapshot, "logs"), filepath.Join(snapshot, "artifacts")); err != nil {
		t.Fatal(err)
	}
	if err := writeSnapshotFiles(snapshot, session); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(snapshot, "logs", "001-combined.log"), []byte("alice /Users/alice/repos/demo sk_abcd1234567890\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(snapshot, "artifacts", "001-failure.txt"), []byte("host=Alice-MBP.local\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	bundlePath, err := createBundle(session.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	contents := unzipContents(t, bundlePath)
	for name, body := range contents {
		if !strings.HasPrefix(name, "capsule/"+session.ID+"/") {
			continue
		}
		if strings.Contains(body, "alice") || strings.Contains(body, "/Users/alice") || strings.Contains(body, "Alice-MBP.local") || strings.Contains(body, "sk_abcd1234567890") {
			t.Fatalf("redacted bundle still contains sensitive content in %s: %q", name, body)
		}
	}
	if !strings.Contains(contents["capsule/"+session.ID+"/logs/001-combined.log"], "[REDACTED") {
		t.Fatal("expected redacted marker in combined log")
	}
}

func TestImportBundleRestoresSnapshot(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	tmp := t.TempDir()
	if err := os.Chdir(tmp); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(wd)
	})

	session := Session{
		ID:        "cap_import",
		StartedAt: time.Now(),
		Git: GitMetadata{
			SHA:    "abcdef1234567890",
			Branch: "main",
		},
		Commands: []CommandRecord{
			{Index: 1, Command: "go test ./...", ExitCode: 1, Logs: CommandLogs{Combined: "logs/001-combined.log"}},
		},
	}
	snapshot := capsuleSnapshotDir(session.ID)
	if err := ensureDirs(filepath.Join(snapshot, "logs")); err != nil {
		t.Fatal(err)
	}
	if err := writeSnapshotFiles(snapshot, session); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(snapshot, "logs", "001-combined.log"), []byte("failed\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	bundlePath, err := createBundle(session.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(snapshot); err != nil {
		t.Fatal(err)
	}

	importedID, err := importBundle(bundlePath)
	if err != nil {
		t.Fatal(err)
	}
	if importedID != session.ID {
		t.Fatalf("imported id = %q, want %q", importedID, session.ID)
	}
	if _, err := os.Stat(filepath.Join(snapshot, "manifest.json")); err != nil {
		t.Fatalf("imported manifest missing: %v", err)
	}
}

func unzipContents(t *testing.T, path string) map[string]string {
	t.Helper()
	reader, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()

	out := make(map[string]string, len(reader.File))
	for _, file := range reader.File {
		rc, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatal(err)
		}
		out[file.Name] = string(data)
	}
	return out
}
