package main

import (
	"regexp"
	"time"
)

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
