package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

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
