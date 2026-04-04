package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfigIncludesAPIFirstModeAndCommonSkillDirs(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Provider != "api" {
		t.Fatalf("provider = %q, want %q", cfg.Provider, "api")
	}

	if cfg.ProviderMode != ProviderModeProviderFirst {
		t.Fatalf("provider_mode = %q, want %q", cfg.ProviderMode, ProviderModeProviderFirst)
	}

	apiCfg := cfg.GetProviderConfig("api")
	if apiCfg.Endpoint != "https://generativelanguage.googleapis.com/v1beta" {
		t.Fatalf("default api endpoint = %q", apiCfg.Endpoint)
	}
	if apiCfg.APIKeyEnv != "GEMINI_API_KEY" {
		t.Fatalf("default api key env = %q", apiCfg.APIKeyEnv)
	}
	if apiCfg.Model != "gemini-embedding-001" {
		t.Fatalf("default api model = %q", apiCfg.Model)
	}

	wantDirs := []string{
		"~/.gemini/antigravity/skills",
		"~/.agents/skills",
		"~/.config/opencode/skills",
		"~/.qwen/skills",
		"~/.cc-switch/skills",
		"~/.codex/skills",
		"~/.codex/vendor_imports/skills",
		"./.agent/skills",
	}

	for _, want := range wantDirs {
		found := false
		for _, dir := range cfg.SkillDirs {
			if dir == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("default skill dirs missing %q", want)
		}
	}

	wantIgnored := []string{
		".git",
		".agency-agents-repo",
		".kdense-repo",
		".superpowers-repo",
	}
	for _, want := range wantIgnored {
		found := false
		for _, name := range cfg.IgnoreDirNames {
			if name == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("default ignore dirs missing %q", want)
		}
	}
}

func TestLoadAddsCommonSkillDirsAndDefaultsToProviderFirst(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "config.json")

	data := []byte(`{
  "skill_dirs": ["~/.gemini/antigravity/skills"],
  "provider": "api"
}`)
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("write temp config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.ProviderMode != ProviderModeProviderFirst {
		t.Fatalf("provider_mode = %q, want %q", cfg.ProviderMode, ProviderModeProviderFirst)
	}

	wantDirs := []string{
		"~/.config/opencode/skills",
		"~/.qwen/skills",
		"~/.cc-switch/skills",
		"~/.codex/skills",
		"~/.codex/vendor_imports/skills",
	}

	for _, want := range wantDirs {
		found := false
		for _, dir := range cfg.SkillDirs {
			if dir == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("loaded skill dirs missing %q", want)
		}
	}

	wantIgnored := []string{
		".git",
		".agency-agents-repo",
		".kdense-repo",
		".superpowers-repo",
	}
	for _, want := range wantIgnored {
		found := false
		for _, name := range cfg.IgnoreDirNames {
			if name == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("loaded ignore dirs missing %q", want)
		}
	}
}

func TestLoadFillsMissingAPIDefaults(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "config.json")

	data := []byte(`{
  "provider": "api",
  "providers": {
    "api": {
      "model": "gemini-embedding-2-preview"
    }
  }
}`)
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("write temp config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	apiCfg := cfg.GetProviderConfig("api")
	if apiCfg.Endpoint != "https://generativelanguage.googleapis.com/v1beta" {
		t.Fatalf("endpoint = %q", apiCfg.Endpoint)
	}
	if apiCfg.APIKeyEnv != "GEMINI_API_KEY" {
		t.Fatalf("api_key_env = %q", apiCfg.APIKeyEnv)
	}
	if apiCfg.Model != "gemini-embedding-2-preview" {
		t.Fatalf("model = %q", apiCfg.Model)
	}
}

func TestExpandedSourcesExpandsTildePaths(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Sources = []ManagedSource{
		{Name: "skills", Path: "~/work/skills"},
		{Name: "repo", Path: "/tmp/repo"},
	}

	expanded := cfg.ExpandedSources()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("user home dir: %v", err)
	}

	want := filepath.Join(home, "work", "skills")
	if expanded[0].Path != want {
		t.Fatalf("expanded[0].Path = %q, want %q", expanded[0].Path, want)
	}
	if expanded[1].Path != "/tmp/repo" {
		t.Fatalf("expanded[1].Path = %q, want %q", expanded[1].Path, "/tmp/repo")
	}
}
