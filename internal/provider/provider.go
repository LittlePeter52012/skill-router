// Package provider defines the pluggable AI reranking interface.
// All backends (keyword passthrough, ONNX local inference, API calls)
// implement the Provider interface for unified skill routing.
//
// Architecture:
//
//	Layer 1 (Default): Local keyword matching — always runs, ~3ms, zero dependencies.
//	Layer 2 (Optional): AI Reranking via Provider interface — ONNX, Gemini, or any
//	                     OpenAI-compatible API endpoint.
//	Layer 3: Fusion ranking — blends keyword + AI scores (configurable weights).
//
// Usage:
//
//	p := provider.Get("local")       // Keyword passthrough
//	p := provider.Get("api")         // API-based reranking
//	results, err := p.Rerank(candidates, query)
package provider

import (
	"github.com/skrt-dev/skill-router/internal/config"
	"github.com/skrt-dev/skill-router/internal/matcher"
)

// Provider defines the unified interface for skill match reranking.
// All backends (keyword passthrough, ONNX, API) implement this interface.
type Provider interface {
	// Name returns the provider's identifier (e.g., "local", "api", "onnx").
	Name() string

	// Rerank takes pre-filtered keyword candidates and returns reranked results.
	// The query parameter is the user's original search query.
	// Implementations may return the candidates unchanged (passthrough),
	// reorder them using embeddings, or apply any custom ranking logic.
	Rerank(candidates []matcher.Result, query string) ([]matcher.Result, error)

	// Available checks if this provider is ready to use.
	// Returns true if all prerequisites are met (model downloaded, API key set, etc.).
	Available() bool
}

// Get returns the appropriate provider for the given configuration.
// Falls back to "local" if the requested provider is unavailable.
func Get(cfg *config.Config) Provider {
	name := cfg.Provider
	if name == "" {
		name = "local"
	}

	switch name {
	case "local":
		return &LocalProvider{}
	case "api":
		pc := cfg.GetProviderConfig("api")
		fc := cfg.GetFusion()
		return NewAPIProvider(pc, fc)
	default:
		return &LocalProvider{}
	}
}

// GetWithFallback returns the requested provider, falling back to local
// if the requested provider is unavailable. This ensures graceful degradation.
func GetWithFallback(cfg *config.Config) Provider {
	p := Get(cfg)
	if !p.Available() {
		return &LocalProvider{}
	}
	return p
}

// ResolveForQuery chooses the provider for a single query.
// Explicit provider flags always win. Otherwise SKRT stays local-first
// unless the config is explicitly set to provider-first mode.
func ResolveForQuery(cfg *config.Config, requested string) Provider {
	if requested != "" {
		override := *cfg
		override.Provider = requested
		return GetWithFallback(&override)
	}

	if cfg.ProviderMode == config.ProviderModeProviderFirst {
		return GetWithFallback(cfg)
	}

	return &LocalProvider{}
}
