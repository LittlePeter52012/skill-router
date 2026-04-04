---
name: skill-router
description: "ALWAYS LOADED. Intelligent skill search router. When no installed skill matches the user's request, or when the user explicitly asks to find/search for a skill, run: skrt query '<user request>'. Read the top result's SKILL.md and follow its instructions. 智能技能路由器，当找不到匹配技能时自动搜索。触发词: find skill, search skill, 查找技能, 搜索技能, skillmux, skrt."
---

# SKRT — Skill Router 智能技能搜索路由器

This meta-skill enables automatic skill discovery from your entire skill library (300+ skills) in **< 50ms**.

## When to Use

Use this skill when:
1. The user's request doesn't match any currently loaded skill
2. The user explicitly asks to find or search for a skill
3. You need to discover which skill handles a specific task

## How to Use

### Step 1: Search

Run the following command with the user's request as the query:

```bash
skrt query "<user's request or task description>"
```

**Examples:**
```bash
skrt query "NotebookLM search"
skrt query "用NotebookLM查找资料"
skrt query "PDF merge files"
skrt query "write scientific paper"
skrt query "brainstorm ideas"
skrt query "molecular docking" --top 10
```

### Step 2: Parse Results

The command outputs JSON with matched skills sorted by relevance:

```json
{
  "query": "...",
  "elapsed_ms": 3.2,
  "provider": "local",
  "results": [
    {
      "rank": 1,
      "name": "skill-name",
      "score": 95,
      "path": "/absolute/path/to/SKILL.md",
      "summary": "Brief description..."
    }
  ]
}
```

### Step 3: Load the Best Match

1. Take the **top result's `path`** field
2. Read that SKILL.md file using `view_file`
3. Follow the instructions in that SKILL.md to complete the user's task

### Step 4: If No Results

If `results` is empty or scores are very low (< 30):
- The skill library doesn't have a matching skill
- Proceed with your general knowledge
- Suggest the user install a relevant skill

## Configuration

```bash
# Pin high-priority skills (always boosted in results)
skrt pin add brainstorming
skrt pin add prompt-master

# Check status
skrt status

# Force index rebuild after installing new skills
skrt index --force
```

## Notes

- The `skrt` binary is installed at `~/go/bin/skrt`
- Config: `~/.skrt/config.json`
- Index cache: `~/.skrt/index.json`
- Supports Chinese (CJK), English, and mixed queries
- Typical latency: < 80ms end-to-end
- Codebase: `~/path/to/skill-router/` (GitHub: `skrt-dev/skill-router`)
