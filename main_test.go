package main

import "testing"

func TestArtifactKind(t *testing.T) {
	cases := map[string]string{
		"app/build/outputs/apk/debug/app-debug.apk":      "android-apk",
		"build/test-results/test/TEST-AppTest.xml":       "junit-xml",
		"target/surefire-reports/junit-results.xml":      "junit-xml",
		"reports/screenshots/login-failure.png":          "screenshot",
		"logs/build.log":                                 "log",
		"ios/build/app.ipa":                              "ios-ipa",
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
