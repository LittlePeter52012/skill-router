// Package matcher implements the multi-strategy skill matching engine.
// It scores each skill against a query using 7 strategies:
//   1. Exact name match (100 pts)
//   2. Name/query containment (90 pts)
//   3. Direct description substring (up to 95 pts)
//   4. Token overlap (up to 80 pts)
//   5. Individual token in description (up to 92 pts)
//   6. Fuzzy Levenshtein matching (up to 40 pts)
//   7. CJK bigram matching (up to 75 pts)
package matcher

import (
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/skrt-dev/skill-router/internal/config"
	"github.com/skrt-dev/skill-router/internal/index"
	"github.com/skrt-dev/skill-router/internal/unicode"
)

// Result represents a matched skill with its computed score.
type Result struct {
	Rank        int    `json:"rank"`
	Name        string `json:"name"`
	Score       int    `json:"score"`
	Path        string `json:"path"`
	Summary     string `json:"summary"`
	MatchReason string `json:"match_reason"`
}

// Engine performs multi-strategy matching against a skill index.
type Engine struct {
	cfg *config.Config
}

// NewEngine creates a new matching engine with the given configuration.
func NewEngine(cfg *config.Config) *Engine {
	return &Engine{cfg: cfg}
}

// Query matches the input query against all indexed skills.
// Returns deduplicated results sorted by score descending, limited to topN.
func (e *Engine) Query(idx *index.Index, query string) []Result {
	if len(idx.Entries) == 0 || query == "" {
		return nil
	}

	queryLower := strings.ToLower(strings.TrimSpace(query))
	queryTokens := queryTokenize(queryLower)
	queryRunes := []rune(queryLower)

	// Deduplicate by skill name: keep the highest-scoring entry
	bestByName := make(map[string]Result)

	for _, entry := range idx.Entries {
		score, reason := e.score(entry, queryLower, queryTokens, queryRunes)

		// Apply pinned boost:
		// - If pinned AND matched query: +50 bonus (reward relevance)
		// - If pinned but NOT matched: set to min_score (ensure visibility)
		isPinned := e.cfg.IsPinned(entry.Name) || e.cfg.IsPinned(entry.Dir)
		if isPinned {
			if score > 0 {
				score += 50
				reason += "+pinned"
			} else {
				score = e.cfg.MinScore
				reason = "pinned"
			}
		}

		// Apply custom weight boost
		w := e.cfg.GetWeight(entry.Name)
		if w == 0 {
			w = e.cfg.GetWeight(entry.Dir)
		}
		if w > 0 {
			score += w
		}

		if score < e.cfg.MinScore {
			continue
		}

		summary := entry.Description
		if len(summary) > 120 {
			summary = summary[:120] + "..."
		}

		result := Result{
			Name:        entry.Name,
			Score:       score,
			Path:        entry.Path,
			Summary:     summary,
			MatchReason: reason,
		}

		// Deduplicate: keep the entry with the highest score for each skill name
		if existing, ok := bestByName[entry.Name]; !ok || score > existing.Score {
			bestByName[entry.Name] = result
		}
	}

	// Collect deduplicated results
	results := make([]Result, 0, len(bestByName))
	for _, r := range bestByName {
		results = append(results, r)
	}

	// Sort by score descending, then by name ascending for ties
	sort.Slice(results, func(i, j int) bool {
		if results[i].Score != results[j].Score {
			return results[i].Score > results[j].Score
		}
		return results[i].Name < results[j].Name
	})

	// Apply topN limit
	topN := e.cfg.TopN
	if topN <= 0 {
		topN = 5
	}
	if len(results) > topN {
		results = results[:topN]
	}

	// Set ranks
	for i := range results {
		results[i].Rank = i + 1
	}

	return results
}

// score computes the match score for a single skill entry against the query.
// Returns the best score across all 7 strategies and the match reason.
func (e *Engine) score(entry index.SkillEntry, queryLower string, queryTokens []string, queryRunes []rune) (int, string) {
	bestScore := 0
	bestReason := ""

	nameLower := strings.ToLower(entry.Name)
	dirLower := strings.ToLower(entry.Dir)
	descLower := strings.ToLower(entry.Description)

	// Strategy 1: Exact name match (100 points)
	if nameLower == queryLower || dirLower == queryLower {
		return 100, "exact_name"
	}

	// Strategy 2: Name contained in query or query contained in name (90 points)
	if strings.Contains(queryLower, nameLower) || strings.Contains(queryLower, dirLower) {
		if 90 > bestScore {
			bestScore = 90
			bestReason = "name_in_query"
		}
	}
	if strings.Contains(nameLower, queryLower) || strings.Contains(dirLower, queryLower) {
		if 90 > bestScore {
			bestScore = 90
			bestReason = "query_in_name"
		}
	}

	// Strategy 3: Full query substring in description (up to 95 points)
	// Longer matching substrings = higher score (rewards specificity).
	{
		noSpaceQuery := strings.ReplaceAll(queryLower, " ", "")
		if len(noSpaceQuery) >= 3 && strings.Contains(descLower, noSpaceQuery) {
			directScore := min(95, 60+len(noSpaceQuery)*3)
			if directScore > bestScore {
				bestScore = directScore
				bestReason = "direct_desc_match"
			}
		}
		// Also try query with spaces
		if len(queryLower) >= 3 && strings.Contains(descLower, queryLower) {
			directScore := min(95, 60+len(queryLower)*3)
			if directScore > bestScore {
				bestScore = directScore
				bestReason = "direct_desc_match"
			}
		}
		// Check name/dir containment for each significant query token
		if len(noSpaceQuery) >= 3 && (strings.Contains(nameLower, noSpaceQuery) || strings.Contains(dirLower, noSpaceQuery)) {
			if 92 > bestScore {
				bestScore = 92
				bestReason = "query_in_name"
			}
		}
	}

	// Strategy 4: Token overlap with pre-computed index tokens (up to 80 points)
	if len(queryTokens) > 0 && len(entry.Tokens) > 0 {
		overlap := tokenOverlap(queryTokens, entry.Tokens)
		if overlap > 0 {
			tokenScore := min(80, 30+(overlap*50/len(queryTokens)))
			if tokenScore > bestScore {
				bestScore = tokenScore
				bestReason = "keyword_match"
			}
		}
	}

	// Strategy 5: Individual token substring in description (up to 92 points)
	// Longer tokens score higher — rewards specificity.
	for _, qt := range queryTokens {
		if len(qt) < 2 {
			continue
		}
		if strings.Contains(descLower, qt) {
			subScore := 25 + len(qt)*6
			if len(qt) >= 8 {
				subScore += 10 // Bonus for very specific terms
			}
			subScore = min(92, subScore)
			if subScore > bestScore {
				bestScore = subScore
				bestReason = "desc_substring"
			}
		}
		if strings.Contains(nameLower, qt) || strings.Contains(dirLower, qt) {
			subScore := min(92, 40+len(qt)*6)
			if subScore > bestScore {
				bestScore = subScore
				bestReason = "name_substring"
			}
		}
	}

	// Strategy 6: Fuzzy match using Levenshtein distance (up to 40 points)
	for _, qt := range queryTokens {
		if len(qt) < 3 {
			continue
		}
		nameParts := strings.FieldsFunc(nameLower, func(r rune) bool {
			return r == '-' || r == '_' || r == ' '
		})
		for _, np := range nameParts {
			dist := levenshtein(qt, np)
			maxLen := max(len(qt), len(np))
			if maxLen > 0 && dist <= maxLen/3 {
				similarity := 100 - (dist * 100 / maxLen)
				fuzzyScore := similarity * 40 / 100
				if fuzzyScore > bestScore {
					bestScore = fuzzyScore
					bestReason = "fuzzy_name"
				}
			}
		}
	}

	// Strategy 7: CJK bigram matching (up to 75 points)
	queryBigrams := extractCJKBigrams(queryRunes)
	if len(queryBigrams) > 0 {
		descRunes := []rune(descLower)
		descBigrams := extractCJKBigrams(descRunes)
		if len(descBigrams) > 0 {
			overlap := bigramOverlap(queryBigrams, descBigrams)
			if overlap > 0 {
				cjkScore := min(75, 30+(overlap*45/len(queryBigrams)))
				if cjkScore > bestScore {
					bestScore = cjkScore
					bestReason = "cjk_match"
				}
			}
		}
	}

	return bestScore, bestReason
}

// queryTokenize splits a query into tokens for matching.
// It handles mixed CJK/Latin text by splitting at script boundaries.
func queryTokenize(query string) []string {
	tokenSet := make(map[string]bool)

	// Phase 1: Split at script boundaries (CJK ↔ Latin)
	runes := []rune(query)
	var current []rune
	var lastIsCJK *bool // nil = start

	for _, r := range runes {
		rc := unicode.IsCJK(r)
		isAlphaNum := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')

		if !isAlphaNum && !rc {
			// Delimiter — flush current
			if len(current) > 0 {
				tokenSet[string(current)] = true
				current = nil
			}
			lastIsCJK = nil
			continue
		}

		// Detect script change (CJK → Latin or Latin → CJK)
		if lastIsCJK != nil && *lastIsCJK != rc {
			if len(current) > 0 {
				tokenSet[string(current)] = true
				current = nil
			}
		}

		current = append(current, r)
		lastIsCJK = &rc
	}
	if len(current) > 0 {
		tokenSet[string(current)] = true
	}

	// Phase 2: Also add the full query (no spaces) as a token
	noSpace := strings.ReplaceAll(query, " ", "")
	if len(noSpace) > 1 {
		tokenSet[noSpace] = true
	}

	// Phase 3: Split by spaces/punctuation as well
	words := strings.FieldsFunc(query, func(r rune) bool {
		return r == ' ' || r == ',' || r == '.' || r == '/' || r == '\\' ||
			r == '(' || r == ')' || r == '[' || r == ']' || r == '"' || r == '\''
	})
	for _, w := range words {
		wl := strings.ToLower(w)
		if len(wl) > 0 {
			tokenSet[wl] = true
		}
	}

	// Phase 4: Extract individual CJK characters (unigrams) for partial matching
	for _, r := range runes {
		if unicode.IsCJK(r) {
			tokenSet[string(r)] = true
		}
	}

	tokens := make([]string, 0, len(tokenSet))
	for t := range tokenSet {
		tokens = append(tokens, t)
	}
	return tokens
}

// tokenOverlap counts how many query tokens appear in the entry tokens.
func tokenOverlap(queryTokens, entryTokens []string) int {
	entrySet := make(map[string]bool, len(entryTokens))
	for _, t := range entryTokens {
		entrySet[t] = true
	}

	count := 0
	for _, qt := range queryTokens {
		if entrySet[qt] {
			count++
			continue
		}
		// Check if any entry token contains the query token
		for _, et := range entryTokens {
			if strings.Contains(et, qt) || strings.Contains(qt, et) {
				count++
				break
			}
		}
	}
	return count
}

// extractCJKBigrams extracts consecutive CJK character pairs.
func extractCJKBigrams(runes []rune) []string {
	var bigrams []string
	for i := 0; i < len(runes)-1; i++ {
		if unicode.IsCJK(runes[i]) && unicode.IsCJK(runes[i+1]) {
			bigrams = append(bigrams, string(runes[i:i+2]))
		}
	}
	return bigrams
}

// bigramOverlap counts how many query bigrams appear in the entry bigrams.
func bigramOverlap(queryBigrams, entryBigrams []string) int {
	entrySet := make(map[string]bool, len(entryBigrams))
	for _, b := range entryBigrams {
		entrySet[b] = true
	}
	count := 0
	for _, qb := range queryBigrams {
		if entrySet[qb] {
			count++
		}
	}
	return count
}

// levenshtein computes the edit distance between two strings.
// Uses O(min(m,n)) space with the two-row optimization.
func levenshtein(a, b string) int {
	la := utf8.RuneCountInString(a)
	lb := utf8.RuneCountInString(b)

	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}

	// Ensure a is the shorter string for space optimization
	ra := []rune(a)
	rb := []rune(b)
	if la > lb {
		ra, rb = rb, ra
		la, lb = lb, la
	}

	// Two-row approach
	prev := make([]int, la+1)
	curr := make([]int, la+1)
	for i := 0; i <= la; i++ {
		prev[i] = i
	}

	for j := 1; j <= lb; j++ {
		curr[0] = j
		for i := 1; i <= la; i++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			curr[i] = min(
				prev[i]+1,      // deletion
				curr[i-1]+1,    // insertion
				prev[i-1]+cost, // substitution
			)
		}
		prev, curr = curr, prev
	}

	return prev[la]
}
