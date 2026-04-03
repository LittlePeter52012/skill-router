package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/skrt-dev/skill-router/internal/config"
	"github.com/skrt-dev/skill-router/internal/credentials"
	"github.com/skrt-dev/skill-router/internal/matcher"
)

// APIProvider reranks results using Gemini or any OpenAI-compatible embeddings API.
// Zero external dependencies — uses only net/http and encoding/json.
//
// Supported models:
//   - gemini-embedding-001         (768 dims, fast, recommended for SKRT)
//   - gemini-embedding-2-preview   (3072 dims, multimodal, highest quality)
//   - text-embedding-004           (768 dims, legacy)
//   - text-embedding-3-small       (OpenAI, 1536 dims)
//
// Configuration example (in ~/.skrt/config.json):
//
//	{
//	  "provider": "api",
//	  "providers": {
//	    "api": {
//	      "endpoint": "https://generativelanguage.googleapis.com/v1beta",
//	      "api_key_env": "GEMINI_API_KEY",
//	      "model": "gemini-embedding-001"
//	    }
//	  },
//	  "fusion": {
//	    "keyword_weight": 0.6,
//	    "ai_weight": 0.4,
//	    "timeout_ms": 500
//	  }
//	}
type APIProvider struct {
	config config.ProviderConfig
	fusion config.FusionConfig
}

// NewAPIProvider creates an API provider with the given configurations.
func NewAPIProvider(pc config.ProviderConfig, fc config.FusionConfig) *APIProvider {
	return &APIProvider{config: pc, fusion: fc}
}

// Name returns "api".
func (p *APIProvider) Name() string { return "api" }

// Available checks if the API provider is properly configured.
// Requires an endpoint, a model, and an API key (from env var or credentials file).
func (p *APIProvider) Available() bool {
	if p.config.Endpoint == "" || p.config.APIKeyEnv == "" {
		return false
	}
	key, _ := credentials.Resolve(p.config.APIKeyEnv)
	return key != ""
}

// Rerank uses the configured embedding API to compute semantic similarity
// between the query and each candidate's description, then blends keyword
// scores with embedding similarity using fusion weights.
//
// Flow:
//  1. Batch-embed the query + all candidate descriptions in one API call
//  2. Compute cosine similarity between query embedding and each candidate
//  3. Blend: final_score = keyword_weight * keyword_score + ai_weight * ai_score * 100
//  4. Sort by blended score descending
func (p *APIProvider) Rerank(candidates []matcher.Result, query string) ([]matcher.Result, error) {
	if len(candidates) == 0 {
		return candidates, nil
	}

	apiKey, _ := credentials.Resolve(p.config.APIKeyEnv)
	if apiKey == "" {
		return candidates, fmt.Errorf("API key not found: set $%s or run 'skrt provider setup'.", p.config.APIKeyEnv)
	}

	model := p.config.Model
	if model == "" {
		model = "gemini-embedding-001"
	}

	// Build texts: [query, candidate1_desc, candidate2_desc, ...]
	texts := make([]string, 0, len(candidates)+1)
	texts = append(texts, query)
	for _, c := range candidates {
		// Use name + summary for richer embedding
		desc := c.Name
		if c.Summary != "" {
			desc += ": " + c.Summary
		}
		texts = append(texts, desc)
	}

	// Set timeout from fusion config
	timeoutMs := p.fusion.TimeoutMs
	if timeoutMs == 0 {
		timeoutMs = 500
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutMs)*time.Millisecond)
	defer cancel()

	// Call embedding API
	embeddings, err := p.callEmbeddingAPI(ctx, apiKey, model, texts)
	if err != nil {
		return candidates, fmt.Errorf("embedding API: %w", err)
	}

	if len(embeddings) != len(texts) {
		return candidates, fmt.Errorf("expected %d embeddings, got %d", len(texts), len(embeddings))
	}

	// Compute cosine similarity between query and each candidate
	queryEmb := embeddings[0]
	kwWeight := p.fusion.KeywordWeight
	aiWeight := p.fusion.AIWeight
	if kwWeight == 0 && aiWeight == 0 {
		kwWeight = 0.6
		aiWeight = 0.4
	}

	for i := range candidates {
		similarity := cosineSimilarity(queryEmb, embeddings[i+1])
		// Blend: keyword score (0-100) + ai similarity (0-1 scaled to 0-100)
		kwScore := float64(candidates[i].Score)
		aiScore := similarity * 100.0
		blended := kwWeight*kwScore + aiWeight*aiScore
		candidates[i].Score = int(math.Round(blended))
	}

	// Sort by blended score descending
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Score > candidates[j].Score
	})

	// Re-assign ranks
	for i := range candidates {
		candidates[i].Rank = i + 1
	}

	return candidates, nil
}

// callEmbeddingAPI sends a batch embedding request to the Gemini API.
// Supports Gemini-style endpoint format.
//
// Gemini API format:
//   POST /v1beta/models/{model}:embedContent
//   Header: x-goog-api-key: <key>
//   Body: { "content": { "parts": [{"text": "..."}, ...] }, "taskType": "SEMANTIC_SIMILARITY" }
func (p *APIProvider) callEmbeddingAPI(ctx context.Context, apiKey, model string, texts []string) ([][]float64, error) {
	endpoint := strings.TrimRight(p.config.Endpoint, "/")

	// Detect API style from endpoint
	if strings.Contains(endpoint, "generativelanguage.googleapis.com") {
		return p.callGeminiEmbedding(ctx, endpoint, apiKey, model, texts)
	}
	// OpenAI-compatible fallback
	return p.callOpenAIEmbedding(ctx, endpoint, apiKey, model, texts)
}

// ===== Gemini-style embedding =====

type geminiEmbedRequest struct {
	Content  geminiContent `json:"content"`
	TaskType string        `json:"taskType,omitempty"`
}

type geminiContent struct {
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text string `json:"text"`
}

type geminiEmbedResponse struct {
	Embedding struct {
		Values []float64 `json:"values"`
	} `json:"embedding"`
	Error *struct {
		Message string `json:"message"`
		Code    int    `json:"code"`
	} `json:"error,omitempty"`
}

func (p *APIProvider) callGeminiEmbedding(ctx context.Context, endpoint, apiKey, model string, texts []string) ([][]float64, error) {
	// Use the embedContent API with multiple parts in a single content block.
	// Each part gets its own embedding vector in the response.
	// This is much faster than making N sequential requests.
	url := fmt.Sprintf("%s/models/%s:embedContent", endpoint, model)

	// Build parts array with all texts
	parts := make([]geminiPart, len(texts))
	for i, text := range texts {
		parts[i] = geminiPart{Text: text}
	}

	req := geminiEmbedRequest{
		Content: geminiContent{
			Parts: parts,
		},
		TaskType: "SEMANTIC_SIMILARITY",
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	client := &http.Client{
		Transport: &http.Transport{
			ForceAttemptHTTP2:   true,
			MaxIdleConns:        10,
			IdleConnTimeout:     30 * time.Second,
			DisableKeepAlives:   false,
			TLSHandshakeTimeout: 10 * time.Second,
		},
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-goog-api-key", apiKey)

	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, string(respBody))
	}

	// When multiple parts are sent, Gemini returns a single embedding
	// that is the aggregate. We need individual embeddings per text.
	// Fall back to sequential calls for per-text embeddings.
	var singleResult geminiEmbedResponse
	if err := json.Unmarshal(respBody, &singleResult); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	if singleResult.Error != nil {
		return nil, fmt.Errorf("API error: %s", singleResult.Error.Message)
	}

	// Multi-part content returns ONE embedding → need individual calls
	// Use connection pooling from the same client for speed
	if len(texts) > 1 {
		return p.callGeminiEmbeddingParallel(ctx, client, url, apiKey, texts)
	}

	return [][]float64{singleResult.Embedding.Values}, nil
}

// callGeminiEmbeddingParallel makes concurrent embedding calls with connection pooling.
// All N embeddings run in parallel, reducing total time from N*latency to ~1*latency.
func (p *APIProvider) callGeminiEmbeddingParallel(ctx context.Context, client *http.Client, url, apiKey string, texts []string) ([][]float64, error) {
	type result struct {
		index     int
		embedding []float64
		err       error
	}

	results := make(chan result, len(texts))

	for i, text := range texts {
		go func(idx int, txt string) {
			req := geminiEmbedRequest{
				Content: geminiContent{
					Parts: []geminiPart{{Text: txt}},
				},
				TaskType: "SEMANTIC_SIMILARITY",
			}

			body, err := json.Marshal(req)
			if err != nil {
				results <- result{idx, nil, fmt.Errorf("marshal: %w", err)}
				return
			}

			httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
			if err != nil {
				results <- result{idx, nil, fmt.Errorf("request: %w", err)}
				return
			}
			httpReq.Header.Set("Content-Type", "application/json")
			httpReq.Header.Set("x-goog-api-key", apiKey)

			resp, err := client.Do(httpReq)
			if err != nil {
				results <- result{idx, nil, fmt.Errorf("HTTP: %w", err)}
				return
			}

			respBody, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			if err != nil {
				results <- result{idx, nil, fmt.Errorf("read: %w", err)}
				return
			}

			if resp.StatusCode != http.StatusOK {
				results <- result{idx, nil, fmt.Errorf("API %d: %s", resp.StatusCode, string(respBody))}
				return
			}

			var emb geminiEmbedResponse
			if err := json.Unmarshal(respBody, &emb); err != nil {
				results <- result{idx, nil, fmt.Errorf("parse: %w", err)}
				return
			}

			if emb.Error != nil {
				results <- result{idx, nil, fmt.Errorf("API: %s", emb.Error.Message)}
				return
			}

			results <- result{idx, emb.Embedding.Values, nil}
		}(i, text)
	}

	embeddings := make([][]float64, len(texts))
	for range texts {
		r := <-results
		if r.err != nil {
			return nil, r.err
		}
		embeddings[r.index] = r.embedding
	}

	return embeddings, nil
}

// ===== OpenAI-compatible embedding =====

type openAIEmbedRequest struct {
	Input []string `json:"input"`
	Model string   `json:"model"`
}

type openAIEmbedResponse struct {
	Data []struct {
		Embedding []float64 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (p *APIProvider) callOpenAIEmbedding(ctx context.Context, endpoint, apiKey, model string, texts []string) ([][]float64, error) {
	url := endpoint + "/embeddings"

	req := openAIEmbedRequest{
		Input: texts,
		Model: model,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, string(respBody))
	}

	var result openAIEmbedResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	if result.Error != nil {
		return nil, fmt.Errorf("API error: %s", result.Error.Message)
	}

	// Sort by index to ensure correct ordering
	sort.Slice(result.Data, func(i, j int) bool {
		return result.Data[i].Index < result.Data[j].Index
	})

	embeddings := make([][]float64, len(result.Data))
	for i, d := range result.Data {
		embeddings[i] = d.Embedding
	}

	return embeddings, nil
}

// ===== Math utilities =====

// cosineSimilarity computes the cosine similarity between two vectors.
// Returns a value between -1 and 1, where 1 means identical direction.
func cosineSimilarity(a, b []float64) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}

	var dotProduct, normA, normB float64
	for i := range a {
		dotProduct += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}

	denominator := math.Sqrt(normA) * math.Sqrt(normB)
	if denominator == 0 {
		return 0
	}

	return dotProduct / denominator
}
