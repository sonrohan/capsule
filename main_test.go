package main

import (
	"os"
	"strings"
	"testing"
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
