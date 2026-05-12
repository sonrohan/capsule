package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

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
