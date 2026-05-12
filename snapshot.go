package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

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
