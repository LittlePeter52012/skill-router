package matcher

import (
	"testing"

	"github.com/skrt-dev/skill-router/internal/config"
	"github.com/skrt-dev/skill-router/internal/index"
)

func makeTestIndex() *index.Index {
	return &index.Index{
		Version: 1,
		Entries: []index.SkillEntry{
			{
				Name:        "pdf",
				Description: "Use this skill whenever the user wants to do anything with PDF files.",
				Dir:         "pdf",
				Path:        "/test/skills/pdf/SKILL.md",
				Tokens:      []string{"pdf", "files", "merge", "split", "convert"},
			},
			{
				Name:        "nlm-skill",
				Description: "Google NotebookLM CLI and MCP integration. Use when user mentions NotebookLM, 笔记本LM, or wants to 查找 搜索 knowledge.",
				Dir:         "nlm-skill",
				Path:        "/test/skills/nlm-skill/SKILL.md",
				Tokens:      []string{"nlm", "skill", "notebooklm", "notebook", "google", "查找", "搜索", "笔记", "知识"},
			},
			{
				Name:        "skill-router",
				Description: "ALWAYS LOADED. Intelligent skill search router. Route search requests to the right skill using skrt query.",
				Dir:         "skill-router",
				Path:        "/test/skills/skill-router/SKILL.md",
				Tokens:      []string{"skill", "router", "route", "search", "query"},
			},
			{
				Name:        "brainstorming",
				Description: "You MUST use this before any creative work - creating features, building components.",
				Dir:         "brainstorming",
				Path:        "/test/skills/brainstorming/SKILL.md",
				Tokens:      []string{"brainstorming", "creative", "features", "components", "design"},
			},
			{
				Name:        "scientific-writing",
				Description: "Write scientific manuscripts in full paragraphs. IMRAD structure, citations.",
				Dir:         "scientific-writing",
				Path:        "/test/skills/scientific-writing/SKILL.md",
				Tokens:      []string{"scientific", "writing", "manuscripts", "imrad", "citations", "papers"},
			},
		},
	}
}

func TestExactNameMatch(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.TopN = 10
	cfg.MinScore = 0
	engine := NewEngine(cfg)

	results := engine.Query(makeTestIndex(), "nlm-skill")
	if len(results) == 0 {
		t.Fatal("expected at least one result")
	}
	if results[0].Name != "nlm-skill" {
		t.Errorf("top result = %q, want %q", results[0].Name, "nlm-skill")
	}
	if results[0].Score != 100 {
		t.Errorf("score = %d, want 100", results[0].Score)
	}
}

func TestKeywordMatch(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.TopN = 10
	cfg.MinScore = 0
	engine := NewEngine(cfg)

	results := engine.Query(makeTestIndex(), "PDF merge files")
	if len(results) == 0 {
		t.Fatal("expected at least one result")
	}
	if results[0].Name != "pdf" {
		t.Errorf("top result = %q, want %q", results[0].Name, "pdf")
	}
}

func TestCJKMatch(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.TopN = 10
	cfg.MinScore = 0
	engine := NewEngine(cfg)

	results := engine.Query(makeTestIndex(), "用NotebookLM查找资料")
	if len(results) == 0 {
		t.Fatal("expected at least one result for CJK query")
	}
	if results[0].Name != "nlm-skill" {
		t.Errorf("top result = %q, want %q", results[0].Name, "nlm-skill")
	}
}

func TestPinnedSkillBoost(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.TopN = 10
	cfg.MinScore = 0
	cfg.Pinned = []string{"brainstorming"}
	engine := NewEngine(cfg)

	// Query something unrelated — pinned skill should still appear with min_score
	results := engine.Query(makeTestIndex(), "anything random")
	hasPinned := false
	for _, r := range results {
		if r.Name == "brainstorming" {
			hasPinned = true
			// Pinned but no match = min_score (0 in this config)
			break
		}
	}
	if !hasPinned {
		t.Error("pinned skill 'brainstorming' not found in results")
	}

	// Query something matching + pinned — should get bonus
	results2 := engine.Query(makeTestIndex(), "creative features")
	for _, r := range results2 {
		if r.Name == "brainstorming" {
			if r.Score < 50 {
				t.Errorf("pinned+matched score = %d, want >= 50", r.Score)
			}
			break
		}
	}
}

func TestPinnedRouterDoesNotBeatSpecificSkill(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.TopN = 10
	cfg.MinScore = 0
	cfg.Pinned = []string{"skill-router"}
	engine := NewEngine(cfg)

	results := engine.Query(makeTestIndex(), "NotebookLM search")
	if len(results) == 0 {
		t.Fatal("expected at least one result")
	}
	if results[0].Name != "nlm-skill" {
		t.Fatalf("top result = %q, want %q", results[0].Name, "nlm-skill")
	}
}

func TestEmptyQuery(t *testing.T) {
	cfg := config.DefaultConfig()
	engine := NewEngine(cfg)

	results := engine.Query(makeTestIndex(), "")
	if results != nil {
		t.Errorf("expected nil for empty query, got %d results", len(results))
	}
}

func TestTopNLimit(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.TopN = 2
	cfg.MinScore = 0
	engine := NewEngine(cfg)

	results := engine.Query(makeTestIndex(), "skill")
	if len(results) > 2 {
		t.Errorf("expected <= 2 results, got %d", len(results))
	}
}

func BenchmarkQuery(b *testing.B) {
	cfg := config.DefaultConfig()
	cfg.TopN = 5
	cfg.MinScore = 10
	engine := NewEngine(cfg)
	idx := makeTestIndex()

	// Add more entries to simulate real workload
	for i := 0; i < 200; i++ {
		idx.Entries = append(idx.Entries, index.SkillEntry{
			Name:        "filler-skill-" + string(rune('a'+i%26)),
			Description: "A filler skill for benchmarking with various keywords and descriptions.",
			Dir:         "filler-" + string(rune('a'+i%26)),
			Path:        "/test/skills/filler/SKILL.md",
			Tokens:      []string{"filler", "benchmark", "test"},
		})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		engine.Query(idx, "NotebookLM查找资料")
	}
}
