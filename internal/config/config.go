// Package config handles reading and writing the SKRT configuration file.
// Configuration is stored as JSON at ~/.skrt/config.json and controls
// skill directories, pinned skills, weights, query parameters, and AI provider settings.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	configDir  = ".skrt"
	configFile = "config.json"

	ProviderModeLocalFirst    = "local_first"
	ProviderModeProviderFirst = "provider_first"
)

// ProviderConfig holds settings for a specific AI provider.
type ProviderConfig struct {
	// Endpoint is the API endpoint URL (for API-based providers).
	Endpoint string `json:"endpoint,omitempty"`

	// APIKeyEnv is the environment variable name containing the API key.
	APIKeyEnv string `json:"api_key_env,omitempty"`

	// Model is the model name to use for embeddings or reranking.
	Model string `json:"model,omitempty"`

	// ModelPath is the local filesystem path for ONNX model files.
	ModelPath string `json:"model_path,omitempty"`
}

// FusionConfig controls how keyword and AI scores are combined.
type FusionConfig struct {
	// KeywordWeight is the weight for keyword matching scores (0.0 to 1.0).
	KeywordWeight float64 `json:"keyword_weight"`

	// AIWeight is the weight for AI reranking scores (0.0 to 1.0).
	AIWeight float64 `json:"ai_weight"`

	// TimeoutMs is the maximum time in milliseconds to wait for AI provider response.
	TimeoutMs int `json:"timeout_ms"`
}

// ManagedSource describes a locally checked-out git repository that SKRT can
// update on demand, then optionally run install/sync commands against.
type ManagedSource struct {
	Name     string   `json:"name"`
	Path     string   `json:"path"`
	Install  []string `json:"install,omitempty"`
	Reindex  bool     `json:"reindex,omitempty"`
	Disabled bool     `json:"disabled,omitempty"`
}

// Config holds all user configuration for SKRT.
type Config struct {
	// SkillDirs is a list of directories to scan for SKILL.md files.
	// Supports ~ for home directory expansion.
	SkillDirs []string `json:"skill_dirs"`

	// Pinned is a list of skill names that always receive a score boost (+50).
	Pinned []string `json:"pinned"`

	// Weights maps skill names to custom weight boosts.
	Weights map[string]int `json:"weights"`

	// TopN is the maximum number of results to return.
	TopN int `json:"top_n"`

	// MinScore is the minimum score threshold for results.
	MinScore int `json:"min_score"`

	// IgnoreDirNames skips noisy or duplicate-heavy repository mirrors while indexing.
	IgnoreDirNames []string `json:"ignore_dir_names,omitempty"`

	// Provider is the active provider name: "local", "onnx", or "api".
	// Default is "api" with Gemini embeddings and graceful fallback to local.
	Provider string `json:"provider"`

	// ProviderMode controls whether queries should stay local-first or
	// automatically invoke the configured provider by default.
	ProviderMode string `json:"provider_mode,omitempty"`

	// Providers maps provider names to their configuration.
	Providers map[string]ProviderConfig `json:"providers,omitempty"`

	// Fusion controls how keyword and AI scores are blended.
	Fusion *FusionConfig `json:"fusion,omitempty"`

	// Sources are locally checked-out skill repositories that SKRT can update.
	Sources []ManagedSource `json:"sources,omitempty"`
}

// DefaultConfig returns a sensible default configuration.
func DefaultConfig() *Config {
	return &Config{
		SkillDirs:      defaultSkillDirs(),
		Pinned:         []string{},
		Weights:        map[string]int{},
		TopN:           5,
		MinScore:       10,
		IgnoreDirNames: defaultIgnoreDirNames(),
		Provider:       "api",
		ProviderMode:   ProviderModeProviderFirst,
		Providers: map[string]ProviderConfig{
			"api": defaultAPIProviderConfig(),
		},
		Sources: []ManagedSource{},
	}
}

// ConfigPath returns the default path for the configuration file.
func ConfigPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, configDir, configFile)
}

// Load reads the configuration from disk. If the file doesn't exist,
// returns the default configuration.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultConfig(), nil
		}
		return nil, fmt.Errorf("read config: %w", err)
	}

	cfg := DefaultConfig()
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	// Ensure defaults for zero-value fields
	if cfg.TopN == 0 {
		cfg.TopN = 5
	}
	if cfg.Weights == nil {
		cfg.Weights = map[string]int{}
	}
	if cfg.Provider == "" {
		cfg.Provider = "api"
	}
	if cfg.ProviderMode == "" {
		cfg.ProviderMode = ProviderModeProviderFirst
	}
	cfg.applyProviderDefaults()
	if len(cfg.IgnoreDirNames) == 0 {
		cfg.IgnoreDirNames = defaultIgnoreDirNames()
	} else {
		cfg.IgnoreDirNames = mergeUnique(cfg.IgnoreDirNames, defaultIgnoreDirNames())
	}
	cfg.SkillDirs = mergeUnique(cfg.SkillDirs, defaultSkillDirs())

	return cfg, nil
}

// Save writes the configuration to disk, creating directories as needed.
func Save(path string, cfg *Config) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	return nil
}

// AddPin adds a skill name to the pinned list if not already present.
func (c *Config) AddPin(name string) bool {
	name = strings.TrimSpace(name)
	for _, p := range c.Pinned {
		if p == name {
			return false // Already pinned
		}
	}
	c.Pinned = append(c.Pinned, name)
	return true
}

// RemovePin removes a skill name from the pinned list.
func (c *Config) RemovePin(name string) bool {
	name = strings.TrimSpace(name)
	for i, p := range c.Pinned {
		if p == name {
			c.Pinned = append(c.Pinned[:i], c.Pinned[i+1:]...)
			return true
		}
	}
	return false
}

// IsPinned checks if a skill name is in the pinned list.
func (c *Config) IsPinned(name string) bool {
	for _, p := range c.Pinned {
		if p == name {
			return true
		}
	}
	return false
}

// GetWeight returns the custom weight for a skill, defaulting to 0.
func (c *Config) GetWeight(name string) int {
	if w, ok := c.Weights[name]; ok {
		return w
	}
	return 0
}

// ExpandedSkillDirs returns skill directories with ~ expanded to home dir.
func (c *Config) ExpandedSkillDirs() []string {
	home, _ := os.UserHomeDir()
	dirs := make([]string, 0, len(c.SkillDirs))
	for _, d := range c.SkillDirs {
		if strings.HasPrefix(d, "~/") {
			d = filepath.Join(home, d[2:])
		}
		dirs = append(dirs, d)
	}
	return dirs
}

// GetProviderConfig returns the configuration for the named provider.
// Returns an empty config if the provider is not configured.
func (c *Config) GetProviderConfig(name string) ProviderConfig {
	if c.Providers == nil {
		return ProviderConfig{}
	}
	return c.Providers[name]
}

// GetFusion returns the fusion config with defaults applied.
func (c *Config) GetFusion() FusionConfig {
	if c.Fusion != nil {
		f := *c.Fusion
		if f.KeywordWeight == 0 && f.AIWeight == 0 {
			f.KeywordWeight = 0.6
			f.AIWeight = 0.4
		}
		if f.TimeoutMs == 0 {
			f.TimeoutMs = 500
		}
		return f
	}
	return FusionConfig{
		KeywordWeight: 0.6,
		AIWeight:      0.4,
		TimeoutMs:     500,
	}
}

// ExpandedSources returns managed sources with ~ expanded in their local paths.
func (c *Config) ExpandedSources() []ManagedSource {
	home, _ := os.UserHomeDir()
	sources := make([]ManagedSource, 0, len(c.Sources))
	for _, src := range c.Sources {
		expanded := src
		if strings.HasPrefix(expanded.Path, "~/") {
			expanded.Path = filepath.Join(home, expanded.Path[2:])
		}
		sources = append(sources, expanded)
	}
	return sources
}

func defaultSkillDirs() []string {
	return []string{
		"~/.gemini/antigravity/skills",
		"~/.agents/skills",
		"~/.config/opencode/skills",
		"~/.qwen/skills",
		"~/.cc-switch/skills",
		"~/.codex/skills",
		"~/.codex/vendor_imports/skills",
		"./.agent/skills",
	}
}

func defaultIgnoreDirNames() []string {
	return []string{
		".git",
		".agency-agents-repo",
		".kdense-repo",
		".superpowers-repo",
	}
}

func mergeUnique(existing, defaults []string) []string {
	seen := make(map[string]struct{}, len(existing)+len(defaults))
	merged := make([]string, 0, len(existing)+len(defaults))

	for _, dir := range existing {
		dir = strings.TrimSpace(dir)
		if dir == "" {
			continue
		}
		if _, ok := seen[dir]; ok {
			continue
		}
		seen[dir] = struct{}{}
		merged = append(merged, dir)
	}

	for _, dir := range defaults {
		if _, ok := seen[dir]; ok {
			continue
		}
		seen[dir] = struct{}{}
		merged = append(merged, dir)
	}

	return merged
}

func defaultAPIProviderConfig() ProviderConfig {
	return ProviderConfig{
		Endpoint:  "https://generativelanguage.googleapis.com/v1beta",
		APIKeyEnv: "GEMINI_API_KEY",
		Model:     "gemini-embedding-001",
	}
}

func (c *Config) applyProviderDefaults() {
	if c.Providers == nil {
		c.Providers = make(map[string]ProviderConfig)
	}

	apiCfg := c.Providers["api"]
	defaults := defaultAPIProviderConfig()
	if apiCfg.Endpoint == "" {
		apiCfg.Endpoint = defaults.Endpoint
	}
	if apiCfg.APIKeyEnv == "" {
		apiCfg.APIKeyEnv = defaults.APIKeyEnv
	}
	if apiCfg.Model == "" {
		apiCfg.Model = defaults.Model
	}
	c.Providers["api"] = apiCfg
}
