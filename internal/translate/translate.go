// Package translate provides lightweight query translation for SKRT.
// When a search query contains non-Latin characters (Chinese, Russian, etc.),
// it calls the Gemini generateContent API to translate the query to English,
// enabling accurate matching against English-language skill descriptions.
//
// Design decisions:
//   - Uses Gemini's fast flash model for low latency (~300ms)
//   - Only triggers when non-Latin characters are detected (zero overhead for English queries)
//   - Returns the original query unchanged if translation fails (graceful degradation)
//   - Reuses the same API key and endpoint from SKRT's existing config
package translate

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
	"unicode"

	"github.com/skrt-dev/skill-router/internal/config"
	"github.com/skrt-dev/skill-router/internal/credentials"
)

// NeedsTranslation returns true if the query contains non-Latin script
// characters that would benefit from translation to English.
// Detects CJK (Chinese/Japanese/Korean) and Cyrillic (Russian/Ukrainian) scripts.
func NeedsTranslation(query string) bool {
	for _, r := range query {
		if isCJK(r) || isCyrillic(r) || isArabic(r) {
			return true
		}
	}
	return false
}

// TranslateQuery translates a non-English query to English using Gemini API.
// Returns the translated query, or the original query if translation fails.
// The second return value indicates whether translation was actually performed.
func TranslateQuery(cfg *config.Config, query string) (translated string, didTranslate bool) {
	if !NeedsTranslation(query) {
		return query, false
	}

	pc := cfg.GetProviderConfig("api")
	if pc.Endpoint == "" || pc.APIKeyEnv == "" {
		return query, false
	}

	apiKey, _ := credentials.Resolve(pc.APIKeyEnv)
	if apiKey == "" {
		return query, false
	}

	endpoint := strings.TrimRight(pc.Endpoint, "/")

	// Use Gemini 3.1 Flash Lite — cheapest and fastest current model for translation
	// Model naming convention: gemini-{major}.{minor}-flash-lite-preview
	model := "gemini-3.1-flash-lite-preview"
	url := fmt.Sprintf("%s/models/%s:generateContent?key=%s", endpoint, model, apiKey)

	// Craft a minimal prompt for translation
	prompt := fmt.Sprintf(
		`Translate the following search query to English. Output ONLY the translated text, nothing else. If the input is already English or contains technical terms, keep them as-is.

Query: %s`, query)

	reqBody := geminiGenRequest{
		Contents: []geminiContentBlock{
			{
				Parts: []geminiTextPart{
					{Text: prompt},
				},
			},
		},
		GenerationConfig: &geminiGenConfig{
			MaxOutputTokens: 100,
			Temperature:     0.1, // Low temperature for deterministic translation
		},
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		fmt.Fprintf(os.Stderr, "skrt-translate: marshal error: %v\n", err)
		return query, false
	}

	// Timeout must be generous — Google API latency from China can be 5-8s
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		fmt.Fprintf(os.Stderr, "skrt-translate: request error: %v\n", err)
		return query, false
	}
	httpReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{
		Transport: &http.Transport{
			ForceAttemptHTTP2:     true,
			TLSHandshakeTimeout:   8 * time.Second,
			ResponseHeaderTimeout: 8 * time.Second,
			TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12},
		},
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		fmt.Fprintf(os.Stderr, "skrt-translate: HTTP error: %v\n", err)
		return query, false
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Fprintf(os.Stderr, "skrt-translate: read error: %v\n", err)
		return query, false
	}

	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "skrt-translate: API error %d: %s\n", resp.StatusCode, string(respBody[:min(200, len(respBody))]))
		return query, false
	}

	var result geminiGenResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return query, false
	}

	// Extract translated text from response
	if len(result.Candidates) > 0 &&
		result.Candidates[0].Content != nil &&
		len(result.Candidates[0].Content.Parts) > 0 {
		translated := strings.TrimSpace(result.Candidates[0].Content.Parts[0].Text)
		if translated != "" && translated != query {
			return translated, true
		}
	}

	return query, false
}

// ===== Unicode detection helpers =====

func isCJK(r rune) bool {
	return (r >= 0x4E00 && r <= 0x9FFF) || // CJK Unified Ideographs
		(r >= 0x3400 && r <= 0x4DBF) || // CJK Extension A
		(r >= 0xF900 && r <= 0xFAFF) || // CJK Compatibility
		(r >= 0x3000 && r <= 0x303F) || // CJK punctuation
		(r >= 0x3040 && r <= 0x309F) || // Hiragana
		(r >= 0x30A0 && r <= 0x30FF) // Katakana
}

func isCyrillic(r rune) bool {
	return unicode.Is(unicode.Cyrillic, r)
}

func isArabic(r rune) bool {
	return unicode.Is(unicode.Arabic, r)
}

// ===== Gemini generateContent API types =====

type geminiGenRequest struct {
	Contents         []geminiContentBlock `json:"contents"`
	GenerationConfig *geminiGenConfig     `json:"generationConfig,omitempty"`
}

type geminiContentBlock struct {
	Parts []geminiTextPart `json:"parts"`
}

type geminiTextPart struct {
	Text string `json:"text"`
}

type geminiGenConfig struct {
	MaxOutputTokens int     `json:"maxOutputTokens,omitempty"`
	Temperature     float64 `json:"temperature,omitempty"`
}

type geminiGenResponse struct {
	Candidates []struct {
		Content *struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content,omitempty"`
	} `json:"candidates,omitempty"`
}
