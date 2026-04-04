package provider

import (
	"math"
	"testing"

	"github.com/skrt-dev/skill-router/internal/config"
	"github.com/skrt-dev/skill-router/internal/matcher"
)

func TestCosineSimilarity(t *testing.T) {
	tests := []struct {
		name     string
		a, b     []float64
		expected float64
	}{
		{"identical", []float64{1, 0, 0}, []float64{1, 0, 0}, 1.0},
		{"orthogonal", []float64{1, 0, 0}, []float64{0, 1, 0}, 0.0},
		{"opposite", []float64{1, 0, 0}, []float64{-1, 0, 0}, -1.0},
		{"similar", []float64{1, 1, 0}, []float64{1, 0, 0}, 1.0 / math.Sqrt(2)},
		{"empty", []float64{}, []float64{}, 0.0},
		{"length_mismatch", []float64{1, 2}, []float64{1, 2, 3}, 0.0},
		{"zero_vector", []float64{0, 0, 0}, []float64{1, 2, 3}, 0.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := cosineSimilarity(tt.a, tt.b)
			if math.Abs(result-tt.expected) > 1e-9 {
				t.Errorf("expected %f, got %f", tt.expected, result)
			}
		})
	}
}

func TestAPIProviderName(t *testing.T) {
	p := NewAPIProvider(config.ProviderConfig{}, config.FusionConfig{})
	if p.Name() != "api" {
		t.Errorf("expected 'api', got '%s'", p.Name())
	}
}

func TestAPIProviderAvailable_NoCreds(t *testing.T) {
	p := NewAPIProvider(config.ProviderConfig{}, config.FusionConfig{})
	if p.Available() {
		t.Error("expected unavailable with empty config")
	}
}

func TestAPIProviderRerank_EmptyCandidates(t *testing.T) {
	p := NewAPIProvider(config.ProviderConfig{}, config.FusionConfig{})
	results, err := p.Rerank(nil, "test")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if results != nil {
		t.Error("expected nil results for nil input")
	}
}

func TestAPIProviderRerank_NoKey(t *testing.T) {
	p := NewAPIProvider(
		config.ProviderConfig{
			Endpoint:  "https://example.com",
			APIKeyEnv: "NONEXISTENT_SKRT_TEST_KEY_12345",
		},
		config.FusionConfig{KeywordWeight: 0.6, AIWeight: 0.4},
	)

	candidates := []matcher.Result{
		{Rank: 1, Name: "test-skill", Score: 90},
	}

	// Should return candidates unchanged with error (graceful fallback)
	results, err := p.Rerank(candidates, "test")
	if err == nil {
		t.Error("expected error for missing API key")
	}
	if len(results) != 1 {
		t.Errorf("expected 1 candidate returned, got %d", len(results))
	}
}

func TestLocalProviderPassthrough(t *testing.T) {
	p := &LocalProvider{}

	if p.Name() != "local" {
		t.Errorf("expected 'local', got '%s'", p.Name())
	}
	if !p.Available() {
		t.Error("local should always be available")
	}

	candidates := []matcher.Result{
		{Rank: 1, Name: "a", Score: 90},
		{Rank: 2, Name: "b", Score: 80},
	}

	results, err := p.Rerank(candidates, "test")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if len(results) != 2 || results[0].Name != "a" {
		t.Error("local should return candidates unchanged")
	}
}

func TestGetProvider(t *testing.T) {
	cfg := config.DefaultConfig()

	// Default should be local
	p := Get(cfg)
	if p.Name() != "local" {
		t.Errorf("expected 'local', got '%s'", p.Name())
	}

	// Unknown should fallback to local
	cfg.Provider = "nonexistent"
	p = Get(cfg)
	if p.Name() != "local" {
		t.Errorf("expected 'local' fallback, got '%s'", p.Name())
	}
}

func TestGetWithFallback(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Provider = "api"
	// API without config should fall back to local
	p := GetWithFallback(cfg)
	if p.Name() != "local" {
		t.Errorf("expected fallback to 'local', got '%s'", p.Name())
	}
}

func TestResolveForQueryPrefersLocalUntilAPIIsExplicit(t *testing.T) {
	t.Setenv("SKRT_TEST_API_KEY", "dummy-key")

	cfg := config.DefaultConfig()
	cfg.Provider = "api"
	cfg.ProviderMode = config.ProviderModeLocalFirst
	cfg.Providers = map[string]config.ProviderConfig{
		"api": {
			Endpoint:  "https://example.com",
			APIKeyEnv: "SKRT_TEST_API_KEY",
		},
	}

	p := ResolveForQuery(cfg, "")
	if p.Name() != "local" {
		t.Fatalf("implicit provider = %q, want %q", p.Name(), "local")
	}

	p = ResolveForQuery(cfg, "api")
	if p.Name() != "api" {
		t.Fatalf("explicit provider = %q, want %q", p.Name(), "api")
	}
}

func TestResolveForQueryUsesConfiguredProviderInProviderFirstMode(t *testing.T) {
	t.Setenv("SKRT_TEST_API_KEY", "dummy-key")

	cfg := config.DefaultConfig()
	cfg.Provider = "api"
	cfg.ProviderMode = config.ProviderModeProviderFirst
	cfg.Providers = map[string]config.ProviderConfig{
		"api": {
			Endpoint:  "https://example.com",
			APIKeyEnv: "SKRT_TEST_API_KEY",
		},
	}

	p := ResolveForQuery(cfg, "")
	if p.Name() != "api" {
		t.Fatalf("provider-first mode returned %q, want %q", p.Name(), "api")
	}
}
