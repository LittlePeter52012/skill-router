# ⚡ SKRT — Skill Router for AI Agents

**Blazing-fast skill routing engine for AI coding agents.**

SKRT discovers, indexes, and routes agent skills in under 50ms — no Python, no heavyweight dependencies, and graceful local fallback when no API key is configured.

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

**🌐 Cross-language translation (Layer 0):**

When a query contains CJK (Chinese/Japanese/Korean) or Cyrillic (Russian/Ukrainian) characters, SKRT automatically translates it to English via Gemini API before matching — enabling accurate skill discovery regardless of query language.

```
Input:  "小红书运营"
Translated → "Xiaohongshu operation"
Output: agency-xiaohongshu-specialist (score: 87)
```

## 🚀 Quick Start

### Install

```bash
# One-liner install (requires Go 1.21+)
go install github.com/LittlePeter52012/skill-router/cmd/skrt@latest

# Or from source
git clone https://github.com/LittlePeter52012/skill-router.git
cd skill-router
make install    # Installs 'skrt' to $GOPATH/bin
```

### New Machine Setup

```bash
# 1. Install SKRT
go install github.com/LittlePeter52012/skill-router/cmd/skrt@latest

# 2. Set up Gemini API key (for AI reranking + cross-language translation)
skrt provider setup   # Interactive — prompts for API key

# 3. Index your skills
skrt index

# 4. Verify everything works
skrt query "frontend developer"      # English
skrt query "小红书运营"                # Chinese (auto-translated)
skrt query "машинное обучение"       # Russian (auto-translated)
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
  "provider": "api",
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
    "~/.config/opencode/skills",
    "~/.qwen/skills",
    "~/.cc-switch/skills",
    "~/.codex/skills",
    "~/.codex/vendor_imports/skills",
    "./.agent/skills"
  ],
  "pinned": ["brainstorming"],
  "weights": { "brainstorming": 20 },
  "top_n": 5,
  "min_score": 10,
  "ignore_dir_names": [
    ".git",
    ".agency-agents-repo",
    ".kdense-repo",
    ".superpowers-repo"
  ],
  "provider": "api",
  "provider_mode": "provider_first",
  "providers": {
    "api": {
      "endpoint": "https://generativelanguage.googleapis.com/v1beta",
      "api_key_env": "GEMINI_API_KEY",
      "model": "gemini-embedding-001"
    }
  },
  "sources": [
    {
      "name": "skill-router",
      "path": "~/src/skill-router",
      "install": ["make install"],
      "reindex": true
    }
  ]
}
```

### Skill Directories

```bash
skrt dir add ~/my-custom-skills
skrt dir remove ~/old-skills
skrt dir list
```

### Ignore Duplicate Mirrors

Use `ignore_dir_names` to skip checked-out repo mirrors inside your skills tree.
This keeps the index lean when a tool installs skills at the top level but also
keeps a hidden source checkout alongside them.

Common defaults:

```json
{
  "ignore_dir_names": [
    ".git",
    ".agency-agents-repo",
    ".kdense-repo",
    ".superpowers-repo"
  ]
}
```

### Pinned Skills

Pinned skills stay visible, but exact/name matches still outrank generic utility pins:

```bash
skrt pin add brainstorming
skrt pin remove brainstorming
skrt pin list
```

### 🧠 Smart Pin (Auto-Suggest)

SKRT can analyze your agent's usage patterns and **automatically suggest which skills to pin**:

```bash
# Interactive: analyze, show suggestions, and ask for confirmation
skrt smart-pin

# Auto-apply: skip confirmation prompt
skrt smart-pin --apply
```

How it works:
1. **Infrastructure Detection** — identifies essential "always loaded" skills
2. **Chat History Analysis** — scans agent conversation logs to find frequently-used skills
3. **Popularity Heuristics** — recognizes universally useful skill categories
4. **Interactive Confirmation** — shows scored suggestions and lets you approve

```
🔍 Analyzing your agent usage patterns...

📊 Found 7 relevant skills from 352 installed:

  1. 🏗️ skill-router (relevance: 40)
     → skill routing infrastructure (this tool)
  2. 🏗️ brainstorming (relevance: 35)
     → pre-work requirement for creative tasks
  3. 📌 writing (relevance: 28)
     → mentioned 9× in chat history
  ...

📌 New pins to add: writing, scientific-writing, prompt-master
Apply these pins? [Y/n]
```

### 🔄 Managed Source Updates

SKRT can also update locally checked-out skill repositories and run the
appropriate sync/install commands for each one.

```bash
# Register a managed source
skrt source add superpowers ~/src/superpowers \
  --install "make install" \
  --reindex

# See what SKRT will update without changing anything
skrt update --dry-run

# Pull all managed sources and rebuild the index if needed
skrt update --reindex
```

This is intentionally generic: you decide which repositories are managed and
which install commands should run after a successful pull.

## 🤖 AI Provider Architecture

SKRT supports pluggable AI backends for enhanced accuracy. New installs default
to **Gemini API-first** with graceful fallback to local keyword ranking when no
API key is available or the API request fails.

| Provider | Speed | Accuracy | Dependencies |
|----------|-------|----------|--------------|
| `api` (default) | ~3-5s | Excellent | API key |
| `local` | ~3ms | Good | None |

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
# Default queries already use the configured API provider
skrt query "protein structure prediction"

# Force local-only matching for a single query
skrt query "protein structure prediction" --provider local

# Explicitly switch the default query mode if needed
skrt provider set api
skrt provider set local

# Graceful fallback: if API fails or no key is set, keyword results are used automatically
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
skrt smart-pin [--apply]      Auto-suggest pins from usage patterns
skrt dir add|remove|list      Manage skill directories
skrt source add|remove|list   Manage git-backed skill sources
skrt provider status          Show AI provider configuration
skrt provider setup           Configure API provider (interactive)
skrt provider set <name>      Switch default query mode: local, api
skrt provider models          List supported embedding models
skrt update [--reindex]       Pull managed sources and optionally rebuild index
skrt version                  Show version info

Options:
  --verbose, -v           Show debug info on stderr
  --top N                 Override max results (default: 5)
  --provider, -p NAME     Use provider for this query only
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
                     │  Layer 0:     │
                     │  Translation  │
                     │  (CJK/Cyrillic│
                     │  → English)   │
                     │               │
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
│   ├── smartpin/      # Usage-based smart pin suggestions
│   ├── translate/     # Cross-language query translation (Gemini API)
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

## 🏪 Publishing & Distribution

SKRT is a **CLI tool + Agent Meta-Skill** — it provides both a human CLI interface and a machine-readable SKILL.md for agent integration.

| Channel | Status | Notes |
|---------|--------|-------|
| **GitHub** | ✅ Active | Source code, releases, issues |
| **go install** | ✅ Available | `go install github.com/skrt-dev/skill-router/cmd/skrt@latest` |
| **Agensi.io** | 🔜 Planned | SKILL.md marketplace (skill zip upload) |
| **agentskills.io** | 🔜 Planned | Open standard skill directory |

### What SKRT Is NOT

- ❌ **Not an MCP server** — MCP requires persistent JSON-RPC processes. SKRT is a one-shot CLI.
- ✅ **Prefer local skills first** — SKRT scans local skill directories (Gemini, Codex, OpenCode, Qwen, cc-switch, etc.) before any optional API reranking.
- ❌ **Not a LobeHub plugin** — LobeHub uses OpenAPI-spec HTTP plugins for LobeChat.
- ❌ **Not an IDE extension** — works across all terminal-based agents.

## License

MIT
