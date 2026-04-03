package provider

import (
	"github.com/skrt-dev/skill-router/internal/matcher"
)

// LocalProvider is a passthrough provider that returns keyword matching results
// unchanged. This is the default provider — zero dependencies, offline, ~3ms.
type LocalProvider struct{}

// Name returns "local".
func (p *LocalProvider) Name() string { return "local" }

// Rerank returns candidates unchanged (passthrough).
// The keyword engine already provides high-quality results.
func (p *LocalProvider) Rerank(candidates []matcher.Result, _ string) ([]matcher.Result, error) {
	return candidates, nil
}

// Available always returns true — local provider has no external dependencies.
func (p *LocalProvider) Available() bool { return true }
