package main

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

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
