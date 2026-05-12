package main

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
)

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
