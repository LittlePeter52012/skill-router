package frontmatter

import (
	"strings"
	"testing"
)

func TestParseSimple(t *testing.T) {
	input := `---
name: test-skill
description: "A test skill for testing purposes."
version: "1.0"
---

# Test Skill Content
`
	meta, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if meta.Name != "test-skill" {
		t.Errorf("name = %q, want %q", meta.Name, "test-skill")
	}
	if meta.Description != "A test skill for testing purposes." {
		t.Errorf("description = %q, want %q", meta.Description, "A test skill for testing purposes.")
	}
}

func TestParseUnquoted(t *testing.T) {
	input := `---
name: my-skill
description: Simple unquoted description
---
`
	meta, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if meta.Name != "my-skill" {
		t.Errorf("name = %q, want %q", meta.Name, "my-skill")
	}
	if meta.Description != "Simple unquoted description" {
		t.Errorf("description = %q, want %q", meta.Description, "Simple unquoted description")
	}
}

func TestParseCJK(t *testing.T) {
	input := `---
name: nlm-skill
description: "Google NotebookLM CLI. 用于查找和搜索知识源。"
---
`
	meta, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if meta.Name != "nlm-skill" {
		t.Errorf("name = %q, want %q", meta.Name, "nlm-skill")
	}
	if !strings.Contains(meta.Description, "查找") {
		t.Errorf("description should contain CJK characters, got %q", meta.Description)
	}
}

func TestParseMultiLine(t *testing.T) {
	input := `---
name: multi-line-skill
description: "This is a very long description
  that spans multiple lines
  and ends here."
---
`
	meta, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if meta.Name != "multi-line-skill" {
		t.Errorf("name = %q, want %q", meta.Name, "multi-line-skill")
	}
	if !strings.Contains(meta.Description, "multiple lines") {
		t.Errorf("description should contain multi-line content, got %q", meta.Description)
	}
}

func TestParseNoFrontmatter(t *testing.T) {
	input := `# Just a markdown file
No frontmatter here.
`
	meta, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if meta.Name != "" {
		t.Errorf("name should be empty, got %q", meta.Name)
	}
}

func TestParseSingleQuotes(t *testing.T) {
	input := `---
name: 'single-quoted'
description: 'Single quoted description'
---
`
	meta, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if meta.Name != "single-quoted" {
		t.Errorf("name = %q, want %q", meta.Name, "single-quoted")
	}
}

func BenchmarkParse(b *testing.B) {
	input := `---
name: benchmark-skill
description: "A skill for benchmarking the frontmatter parser performance with a moderately long description."
version: "1.0"
---

# Benchmark Skill
This is a long content body that should be ignored by the parser.
` + strings.Repeat("More content here.\n", 100)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Parse(strings.NewReader(input))
	}
}
