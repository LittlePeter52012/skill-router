package index

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBuildSkipsIgnoredRepoMirrors(t *testing.T) {
	root := t.TempDir()

	keepDir := filepath.Join(root, "skills", "pdf")
	if err := os.MkdirAll(keepDir, 0755); err != nil {
		t.Fatalf("mkdir keep dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(keepDir, "SKILL.md"), []byte("---\nname: pdf\ndescription: PDF tools\n---\n"), 0644); err != nil {
		t.Fatalf("write keep skill: %v", err)
	}

	ignoredDir := filepath.Join(root, ".agency-agents-repo", "integrations", "antigravity", "agency-pdf")
	if err := os.MkdirAll(ignoredDir, 0755); err != nil {
		t.Fatalf("mkdir ignored dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(ignoredDir, "SKILL.md"), []byte("---\nname: agency-pdf\ndescription: duplicate mirror\n---\n"), 0644); err != nil {
		t.Fatalf("write ignored skill: %v", err)
	}

	idx, err := Build([]string{root}, []string{".agency-agents-repo"})
	if err != nil {
		t.Fatalf("build index: %v", err)
	}

	if len(idx.Entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(idx.Entries))
	}
	if idx.Entries[0].Name != "pdf" {
		t.Fatalf("entry name = %q, want %q", idx.Entries[0].Name, "pdf")
	}
}
