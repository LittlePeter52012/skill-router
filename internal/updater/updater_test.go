package updater

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/skrt-dev/skill-router/internal/config"
)

func TestUpdateSourcePullsAndRunsInstallHooks(t *testing.T) {
	root := t.TempDir()
	remote := filepath.Join(root, "remote.git")
	work := filepath.Join(root, "work")
	clone := filepath.Join(root, "clone")
	marker := filepath.Join(clone, "install-ran.txt")

	run(t, root, "git", "init", "--bare", remote)
	run(t, root, "git", "clone", remote, work)
	run(t, work, "git", "config", "user.email", "test@example.com")
	run(t, work, "git", "config", "user.name", "SKRT Test")
	if err := os.WriteFile(filepath.Join(work, "README.md"), []byte("v1\n"), 0644); err != nil {
		t.Fatalf("write initial file: %v", err)
	}
	run(t, work, "git", "add", "README.md")
	run(t, work, "git", "commit", "-m", "init")
	run(t, work, "git", "push", "origin", "HEAD")

	run(t, root, "git", "clone", remote, clone)

	if err := os.WriteFile(filepath.Join(work, "README.md"), []byte("v2\n"), 0644); err != nil {
		t.Fatalf("write updated file: %v", err)
	}
	run(t, work, "git", "add", "README.md")
	run(t, work, "git", "commit", "-m", "update")
	run(t, work, "git", "push", "origin", "HEAD")

	result, err := UpdateSource(config.ManagedSource{
		Name:    "skills",
		Path:    clone,
		Install: []string{"echo ok > install-ran.txt"},
	}, false)
	if err != nil {
		t.Fatalf("update source: %v", err)
	}

	if !result.Updated {
		t.Fatalf("Updated = false, want true")
	}
	if result.InstallRan != 1 {
		t.Fatalf("InstallRan = %d, want 1", result.InstallRan)
	}
	if result.BeforeRev == result.AfterRev {
		t.Fatalf("before and after revisions should differ")
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("install marker not created: %v", err)
	}
}

func TestUpdateSourceDryRunDoesNotPullOrInstall(t *testing.T) {
	root := t.TempDir()
	remote := filepath.Join(root, "remote.git")
	work := filepath.Join(root, "work")
	clone := filepath.Join(root, "clone")
	marker := filepath.Join(clone, "install-ran.txt")

	run(t, root, "git", "init", "--bare", remote)
	run(t, root, "git", "clone", remote, work)
	run(t, work, "git", "config", "user.email", "test@example.com")
	run(t, work, "git", "config", "user.name", "SKRT Test")
	if err := os.WriteFile(filepath.Join(work, "README.md"), []byte("v1\n"), 0644); err != nil {
		t.Fatalf("write initial file: %v", err)
	}
	run(t, work, "git", "add", "README.md")
	run(t, work, "git", "commit", "-m", "init")
	run(t, work, "git", "push", "origin", "HEAD")

	run(t, root, "git", "clone", remote, clone)

	if err := os.WriteFile(filepath.Join(work, "README.md"), []byte("v2\n"), 0644); err != nil {
		t.Fatalf("write updated file: %v", err)
	}
	run(t, work, "git", "add", "README.md")
	run(t, work, "git", "commit", "-m", "update")
	run(t, work, "git", "push", "origin", "HEAD")

	result, err := UpdateSource(config.ManagedSource{
		Name:    "skills",
		Path:    clone,
		Install: []string{"echo ok > install-ran.txt"},
	}, true)
	if err != nil {
		t.Fatalf("dry-run update source: %v", err)
	}

	if result.Updated {
		t.Fatalf("Updated = true, want false in dry-run")
	}
	if result.InstallRan != 0 {
		t.Fatalf("InstallRan = %d, want 0", result.InstallRan)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("install marker should not exist in dry-run")
	}
}

func run(t *testing.T, dir string, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v failed: %v\n%s", name, args, err, out)
	}
}
