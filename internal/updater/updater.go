package updater

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/skrt-dev/skill-router/internal/config"
)

// Result captures the outcome of updating one managed source.
type Result struct {
	Name          string
	Path          string
	BeforeRev     string
	AfterRev      string
	Updated       bool
	InstallRan    int
	InstallOutput string
}

// UpdateSource pulls the local git repository and runs optional install hooks.
func UpdateSource(src config.ManagedSource, dryRun bool) (Result, error) {
	result := Result{Name: src.Name, Path: src.Path}
	if src.Disabled {
		return result, fmt.Errorf("source %q is disabled", src.Name)
	}
	if src.Path == "" {
		return result, fmt.Errorf("source %q has no path", src.Name)
	}

	gitDir := filepath.Join(src.Path, ".git")
	if _, err := os.Stat(gitDir); err != nil {
		return result, fmt.Errorf("source %q is not a git repo: %w", src.Name, err)
	}

	beforeRev, err := gitOutput(src.Path, "rev-parse", "HEAD")
	if err != nil {
		return result, fmt.Errorf("read current revision: %w", err)
	}
	result.BeforeRev = beforeRev

	if dryRun {
		result.AfterRev = beforeRev
		return result, nil
	}

	if _, err := gitOutput(src.Path, "pull", "--ff-only", "--quiet"); err != nil {
		return result, fmt.Errorf("git pull: %w", err)
	}

	afterRev, err := gitOutput(src.Path, "rev-parse", "HEAD")
	if err != nil {
		return result, fmt.Errorf("read updated revision: %w", err)
	}
	result.AfterRev = afterRev
	result.Updated = beforeRev != afterRev

	if !result.Updated || len(src.Install) == 0 {
		return result, nil
	}

	var installOutput bytes.Buffer
	for _, command := range src.Install {
		command = strings.TrimSpace(command)
		if command == "" {
			continue
		}
		cmd := exec.Command("/bin/sh", "-c", command)
		cmd.Dir = src.Path
		out, err := cmd.CombinedOutput()
		installOutput.Write(out)
		result.InstallRan++
		if err != nil {
			result.InstallOutput = installOutput.String()
			return result, fmt.Errorf("run install command %q: %w", command, err)
		}
	}
	result.InstallOutput = installOutput.String()

	return result, nil
}

func gitOutput(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s: %s", strings.Join(args, " "), strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}
