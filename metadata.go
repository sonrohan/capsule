package main

import (
	"bufio"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

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
