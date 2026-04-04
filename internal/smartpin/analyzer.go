// Package smartpin analyzes agent conversation history and usage patterns
// to automatically suggest skills to pin for quick access.
package smartpin

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/skrt-dev/skill-router/internal/index"
)

// SkillScore tracks how relevant a skill is based on usage analysis.
type SkillScore struct {
	Name     string
	Score    int
	Reasons  []string
	Category string // "infra", "writing", "research", "data", "tools"
}

// Analyze scans agent conversation logs and installed skills to suggest pins.
// It returns skills sorted by relevance score (highest first).
func Analyze(skills []index.SkillEntry) []SkillScore {
	scores := map[string]*SkillScore{}

	// Phase 1: Category-based infrastructure detection
	infraSkills := detectInfrastructure(skills)
	for _, s := range infraSkills {
		scores[s.Name] = &s
	}

	// Phase 2: Scan conversation logs from known agent locations
	mentionCounts := scanConversationLogs()
	for name, count := range mentionCounts {
		if sc, ok := scores[name]; ok {
			sc.Score += count * 3
			sc.Reasons = append(sc.Reasons, fmt.Sprintf("mentioned %d× in chat history", count))
		} else {
			// Only count if this skill actually exists in our index
			for _, skill := range skills {
				if skill.Name == name {
					scores[name] = &SkillScore{
						Name:     name,
						Score:    count * 3,
						Reasons:  []string{fmt.Sprintf("mentioned %d× in chat history", count)},
						Category: "user",
					}
					break
				}
			}
		}
	}

	// Phase 3: Detect popular/essential skills by description patterns
	for _, skill := range skills {
		if _, exists := scores[skill.Name]; exists {
			continue
		}

		desc := strings.ToUpper(skill.Description)
		// "ALWAYS LOADED" or "ALWAYS" + "RUN/USE" skills are infrastructure
		if strings.Contains(desc, "ALWAYS") &&
			(strings.Contains(desc, "LOAD") || strings.Contains(desc, "RUN") || strings.Contains(desc, "USE")) {
			scores[skill.Name] = &SkillScore{
				Name:     skill.Name,
				Score:    30,
				Reasons:  []string{"marked as ALWAYS LOADED in description"},
				Category: "infra",
			}
		}
	}

	// Convert to sorted slice
	result := make([]SkillScore, 0, len(scores))
	for _, s := range scores {
		result = append(result, *s)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Score > result[j].Score
	})

	return result
}

// detectInfrastructure identifies skills that are commonly needed infrastructure.
func detectInfrastructure(skills []index.SkillEntry) []SkillScore {
	// These patterns identify skills that serve as essential infrastructure
	// for most agent workflows, regardless of the user's specific domain.
	infraPatterns := map[string]struct {
		score    int
		reason   string
		category string
	}{
		"skill-router":  {40, "skill routing infrastructure (this tool)", "infra"},
		"brainstorming": {35, "pre-work requirement for creative tasks", "infra"},
		"backup":        {25, "configuration safety infrastructure", "infra"},
		"prompt-master": {20, "prompt engineering utility", "tools"},
		"pdf":           {20, "universal document format skill", "tools"},
	}

	result := []SkillScore{}
	for _, skill := range skills {
		if info, ok := infraPatterns[skill.Name]; ok {
			result = append(result, SkillScore{
				Name:     skill.Name,
				Score:    info.score,
				Reasons:  []string{info.reason},
				Category: info.category,
			})
		}
	}
	return result
}

// scanConversationLogs reads chat history from known agent locations
// and counts skill name mentions.
func scanConversationLogs() map[string]int {
	counts := map[string]int{}
	home, err := os.UserHomeDir()
	if err != nil {
		return counts
	}

	for _, lp := range conversationLogPatterns(home) {
		files, err := filepath.Glob(lp.pattern)
		if err != nil {
			continue
		}

		// Only scan most recent files
		if len(files) > lp.maxScan {
			sort.Slice(files, func(i, j int) bool {
				fi, _ := os.Stat(files[i])
				fj, _ := os.Stat(files[j])
				if fi == nil || fj == nil {
					return false
				}
				return fi.ModTime().After(fj.ModTime())
			})
			files = files[:lp.maxScan]
		}

		for _, f := range files {
			scanFileForSkillMentions(f, counts)
		}
	}

	return counts
}

type logPattern struct {
	pattern string
	maxScan int
}

func conversationLogPatterns(home string) []logPattern {
	return []logPattern{
		// Antigravity conversation logs
		{filepath.Join(home, ".gemini", "antigravity", "brain", "*", ".system_generated", "logs", "overview.txt"), 20},
		// Claude Code project logs
		{filepath.Join(home, ".claude", "projects", "*", "*", "*.md"), 10},
		// Codex transcript history
		{filepath.Join(home, ".codex", "history.jsonl"), 1},
		// Gemini CLI chat exports and temp chat history
		{filepath.Join(home, ".gemini", "tmp", "*", "chats", "*.json"), 20},
	}
}

// scanFileForSkillMentions reads a file and counts skill name mentions.
func scanFileForSkillMentions(path string, counts map[string]int) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}

	// Limit scan to 512KB per file
	content := string(data)
	if len(content) > 512*1024 {
		content = content[:512*1024]
	}

	lower := strings.ToLower(content)

	// Common skill name patterns that indicate actual usage
	skillPatterns := []string{
		"scientific-writing", "writing", "brainstorming", "pdf",
		"whisper-transcribe", "backup", "prompt-master",
		"research-lookup", "scientific-visualization",
		"latex-posters", "scientific-slides",
		"scanpy", "matplotlib", "plotly", "seaborn",
		"statistical-analysis", "hypothesis-generation",
		"generate-image", "scientific-schematics", "infographics",
		"peer-review", "scholar-evaluation",
		"markitdown", "mineru", "immersive-translate",
		"parallel-web", "tavily-cli",
		"todoist-cli", "pyzotero", "citation-management",
		"paper-2-web", "skill-router",
		"dispatching-parallel-agents", "executing-plans",
		"writing-plans", "systematic-debugging",
		"test-driven-development", "verification-before-completion",
	}

	for _, name := range skillPatterns {
		cnt := strings.Count(lower, name)
		if cnt > 0 {
			counts[name] += cnt
		}
	}
}

// FormatSuggestions returns a human-readable string of pin suggestions.
func FormatSuggestions(suggestions []SkillScore, maxShow int) string {
	if len(suggestions) == 0 {
		return "  No suggestions — install more skills or use your agent more to generate data."
	}

	if maxShow > len(suggestions) {
		maxShow = len(suggestions)
	}

	var sb strings.Builder
	for i := 0; i < maxShow; i++ {
		s := suggestions[i]
		icon := categoryIcon(s.Category)
		sb.WriteString(fmt.Sprintf("  %d. %s %s (relevance: %d)\n", i+1, icon, s.Name, s.Score))
		for _, r := range s.Reasons {
			sb.WriteString(fmt.Sprintf("     → %s\n", r))
		}
	}
	return sb.String()
}

func categoryIcon(cat string) string {
	switch cat {
	case "infra":
		return "🏗️"
	case "writing":
		return "✍️"
	case "research":
		return "🔬"
	case "tools":
		return "🔧"
	default:
		return "📌"
	}
}
