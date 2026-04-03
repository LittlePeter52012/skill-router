# ⚡ SKRT — Skill Router for AI Agents

**Blazing-fast skill routing engine for AI coding agents.**

SKRT discovers, indexes, and routes agent skills in under 50ms — no Python, no dependencies, no API keys required.

Works with **Antigravity**, **Claude Code**, **Codex**, **Cursor**, **Gemini CLI**, and any agent that uses [SKILL.md files](https://docs.anthropic.com/en/docs/claude-code/skills).

## ✨ What It Does

AI coding agents like Claude Code and Gemini CLI use **skills** — specialized instruction files that extend their capabilities. With 100+ skills installed, agents waste context budget listing them all. SKRT solves this:

```
Input:  "用NotebookLM查找资料"
Output: nlm-skill (score: 92, 50ms)
```

**7-strategy matching engine:**
1. 🎯 Exact name match (100 pts)
2. 🔗 Name/query containment (90 pts)
3. 📝 Description substring match (up to 95 pts)
4. 🔑 Keyword token overlap (up to 80 pts)
5. 🔍 Individual token search (up to 92 pts)
6. 📐 Fuzzy Levenshtein matching (up to 40 pts)
7. 🀄 CJK bigram matching (up to 75 pts) — full Chinese/Japanese/Korean support

## 🚀 Quick Start

### Install

```bash
# From source (requires Go 1.21+)
git clone https://github.com/skrt-dev/skill-router.git
cd skill-router
make install    # Installs 'skrt' to $GOPATH/bin

# Or build locally
make build      # Binary at ./bin/skrt
```

### First Run

```bash
# Index your skills (~50ms for 200+ skills)
skrt index

# Search for skills
skrt query "PDF merge"
skrt "molecular docking"             # Shorthand
skrt q "用NotebookLM查找" --verbose   # CJK + debug info
```

### Output

```json
{
  "query": "PDF merge",
  "elapsed_ms": 2.3,
  "total": 3,
  "provider": "local",
  "results": [
    {
      "rank": 1,
      "name": "pdf",
      "score": 90,
      "path": "~/.gemini/antigravity/skills/pdf/SKILL.md",
      "summary": "Use this skill for anything with PDF files...",
      "match_reason": "name_in_query"
    }
  ]
}
```

## ⚙️ Configuration

Config file: `~/.skrt/config.json`

```json
{
  "skill_dirs": [
    "~/.gemini/antigravity/skills",
    "~/.agents/skills",
    "./.agent/skills"
  ],
  "pinned": ["brainstorming"],
  "weights": { "brainstorming": 20 },
  "top_n": 5,
  "min_score": 10,
  "provider": "local"
}
```

### Skill Directories

```bash
skrt dir add ~/my-custom-skills
skrt dir remove ~/old-skills
skrt dir list
```

### Pinned Skills

Pinned skills always appear in results with a +50 score boost when they match the query:

```bash
skrt pin add brainstorming
skrt pin remove brainstorming
skrt pin list
```

## 🤖 AI Provider Architecture

SKRT supports pluggable AI backends for enhanced accuracy:

| Provider | Speed | Accuracy | Dependencies |
|----------|-------|----------|--------------|
| `local` (default) | ~3ms | Good | None |
| `api` | ~3-5s | Excellent | API key |

### Quick Setup (Gemini)

```bash
# Interactive setup — prompts for API key, stores securely
skrt provider setup

# Or specify model directly
skrt provider setup --model gemini-embedding-2-preview

# Use with any OpenAI-compatible endpoint
skrt provider setup --endpoint https://api.openai.com/v1 --env OPENAI_API_KEY
```

🔒 **Security**: API keys are stored in `~/.skrt/credentials` with `0600` permissions (owner-only). Config files only store the env var name, never the actual key.

```bash
# Use API provider for a single query
skrt query "protein structure prediction" --provider api

# Graceful fallback: if API fails, keyword results are used automatically
```

### Supported Embedding Models

```bash
skrt provider models    # List all supported models
```

- **Gemini** — `gemini-embedding-001` (recommended), `gemini-embedding-2-preview`
- **OpenAI** — `text-embedding-3-small`, `text-embedding-3-large`
- **Local** — Any Ollama/LM Studio/vLLM model with `/embeddings` endpoint

## 📋 CLI Reference

```
skrt query <terms>            Search for matching skills (alias: q)
skrt <terms>                  Shorthand for 'query' (auto-detected)
skrt index [--force]          Rebuild the skill index (alias: idx)
skrt status                   Show index and config status (alias: st)
skrt pin add|remove|list      Manage pinned skills
skrt dir add|remove|list      Manage skill directories
skrt provider status          Show AI provider configuration
skrt provider setup           Configure API provider (interactive)
skrt provider set <name>      Switch provider: local, api
skrt provider models          List supported embedding models
skrt version                  Show version info

Options:
  --verbose, -v           Show debug info on stderr
  --top N                 Override max results (default: 5)
  --provider, -p NAME     Use specific AI provider
```

## 🔌 Agent Integration

### Antigravity / Claude Code SKILL.md

Add this to your `skill-router/SKILL.md`:

```yaml
---
name: skill-router
description: "ALWAYS LOADED. Route queries to the right skill using: skrt query '<user request>'"
---
```

### How It Works

```
User: "Help me merge PDFs"
  ↓
Agent reads skill-router/SKILL.md
  ↓
Agent runs: skrt query "merge PDFs"
  ↓
SKRT returns: pdf (score: 90)
  ↓
Agent reads pdf/SKILL.md and executes
```

## 🏗️ Architecture

```
┌──────────────┐     ┌───────────────┐     ┌──────────────┐
│  Agent Query  │────▶│  SKRT Engine   │────▶│  JSON Output  │
└──────────────┘     │               │     └──────────────┘
                     │  Layer 1:     │
                     │  7-Strategy   │
                     │  Keyword      │
                     │  Matching     │
                     │  (~3ms)       │
                     │               │
                     │  Layer 2:     │
                     │  AI Reranking │
                     │  (optional)   │
                     │               │
                     │  Layer 3:     │
                     │  Fusion       │
                     │  Scoring      │
                     └───────────────┘
```

## 📦 Project Structure

```
skill-router/
├── cmd/skrt/          # CLI entry point
├── internal/
│   ├── config/        # Configuration management (~/.skrt/)
│   ├── credentials/   # Secure API key storage (0600 permissions)
│   ├── index/         # SKILL.md scanning, caching, checksums
│   ├── matcher/       # 7-strategy matching engine
│   ├── provider/      # Pluggable AI backends (local/api)
│   └── unicode/       # Shared CJK text utilities
├── pkg/frontmatter/   # YAML frontmatter parser
├── Makefile
└── README.md
```

## 🧪 Development

```bash
make test     # Run tests with race detector
make bench    # Run benchmarks
make cover    # Generate coverage report
make lint     # Run go vet
make release  # Build for all platforms
```

## License

MIT
