package main

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

const capsuleDir = ".capsule"

var (
	version = "0.2.0"
	commit  = ""
	date    = ""
)

var showRunFailureGuidance = true

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	var err error
	switch os.Args[1] {
	case "start":
		err = cmdStart()
	case "run":
		err = cmdRun(os.Args[2:])
	case "finish":
		err = cmdFinish()
	case "replay":
		err = cmdReplay(os.Args[2:])
	case "ci":
		err = cmdCI(os.Args[2:])
	case "summary":
		err = cmdSummary(os.Args[2:])
	case "agent":
		err = cmdAgent(os.Args[2:])
	case "bundle":
		err = cmdBundle(os.Args[2:])
	case "import":
		err = cmdImport(os.Args[2:])
	case "list":
		err = cmdList()
	case "ui":
		err = cmdUI(os.Args[2:])
	case "version", "-v", "--version":
		cmdVersion()
	case "help", "-h", "--help":
		usage()
	default:
		err = fmt.Errorf("unknown command %q", os.Args[1])
	}

	if err != nil {
		var commandErr *commandFailedError
		if errors.As(err, &commandErr) {
			if !commandErr.quiet {
				fmt.Fprintln(os.Stderr, "capsule:", commandErr.Error())
			}
			code := commandErr.code
			if code < 1 {
				code = 1
			}
			os.Exit(code)
		}
		fmt.Fprintln(os.Stderr, "capsule:", err)
		os.Exit(1)
	}
}

type commandFailedError struct {
	code  int
	quiet bool
}

func (e *commandFailedError) Error() string {
	return fmt.Sprintf("command exited with code %d", e.code)
}

func usage() {
	fmt.Println(`Capsule tracks replayable execution sessions for Git repos.

Usage:
  capsule start
  capsule run <command> [args...]
  capsule finish
  capsule replay <capsule-id> [--rerun]
  capsule ci <command> [args...]
  capsule summary <capsule-id|--last> [--redact]
  capsule agent <capsule-id|--last> [--redact]
  capsule bundle <capsule-id|--last> [--redact] [--include glob] [--exclude glob] [--no-artifacts]
  capsule import <bundle.zip>
  capsule list
  capsule ui [--port 3000]
  capsule version`)
}

func cmdVersion() {
	fmt.Println(versionString())
}

func versionString() string {
	parts := []string{"capsule", version}
	if commit != "" {
		parts = append(parts, commit)
	}
	if date != "" {
		parts = append(parts, date)
	}
	return strings.Join(parts, " ")
}
