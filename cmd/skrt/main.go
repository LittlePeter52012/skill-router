// skrt — A blazing-fast skill router for AI coding agents.
//
// SKRT (Skill Router) discovers and ranks agent skills using a 7-strategy
// matching engine with optional AI reranking. Works with Antigravity,
// Claude Code, Codex, Cursor, Gemini CLI, and any SKILL.md-based agent.
//
// Usage:
//
//	skrt query "search terms"        Search for matching skills
//	skrt index [--force]             Rebuild the skill index
//	skrt status                      Show index and config status
//	skrt pin add <name>              Pin a skill for priority boosting
//	skrt pin remove <name>           Unpin a skill
//	skrt pin list                    List pinned skills
//	skrt dir add <path>              Add a skill search directory
//	skrt dir remove <path>           Remove a skill search directory
//	skrt dir list                    List skill search directories
//	skrt version                     Show version information
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/skrt-dev/skill-router/internal/config"
	"github.com/skrt-dev/skill-router/internal/credentials"
	"github.com/skrt-dev/skill-router/internal/index"
	"github.com/skrt-dev/skill-router/internal/matcher"
	"github.com/skrt-dev/skill-router/internal/provider"
)

var (
	version   = "0.2.0"
	buildTime = "dev"
	gitCommit = "none"
)

// QueryOutput is the top-level JSON output for query commands.
type QueryOutput struct {
	Query     string           `json:"query"`
	ElapsedMs float64          `json:"elapsed_ms"`
	Total     int              `json:"total"`
	Provider  string           `json:"provider"`
	Results   []matcher.Result `json:"results"`
}

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	cmd := os.Args[1]

	switch cmd {
	case "query", "q":
		cmdQuery()
	case "index", "idx":
		cmdIndex()
	case "status", "st":
		cmdStatus()
	case "pin":
		cmdPin()
	case "dir":
		cmdDir()
	case "provider", "prov":
		cmdProvider()
	case "version", "-v", "--version":
		cmdVersion()
	case "help", "-h", "--help":
		printUsage()
	default:
		// If the first arg doesn't match a command, treat it as a query.
		// This allows: skrt "NotebookLM search"
		os.Args = append([]string{os.Args[0], "query"}, os.Args[1:]...)
		cmdQuery()
	}
}

// cmdQuery implements the "query" subcommand.
func cmdQuery() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "Usage: skrt query <search terms>")
		os.Exit(1)
	}

	start := time.Now()

	// Load config
	cfg, err := config.Load(config.ConfigPath())
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: config load failed: %v, using defaults\n", err)
		cfg = config.DefaultConfig()
	}

	// Parse flags from query args
	verbose := false
	topN := cfg.TopN
	providerName := ""
	cleanArgs := []string{}
	for i := 2; i < len(os.Args); i++ {
		switch os.Args[i] {
		case "--verbose", "-v":
			verbose = true
		case "--top":
			if i+1 < len(os.Args) {
				i++
				fmt.Sscanf(os.Args[i], "%d", &topN)
			}
		case "--provider", "-p":
			if i+1 < len(os.Args) {
				i++
				providerName = os.Args[i]
			}
		case "--json":
			// Default output is JSON, this is a no-op for compatibility
		default:
			cleanArgs = append(cleanArgs, os.Args[i])
		}
	}
	if topN > 0 {
		cfg.TopN = topN
	}
	if providerName != "" {
		cfg.Provider = providerName
	}

	query := strings.Join(cleanArgs, " ")
	if query == "" {
		fmt.Fprintln(os.Stderr, "Error: empty query")
		os.Exit(1)
	}

	// Get or build index
	dirs := cfg.ExpandedSkillDirs()
	idx, err := index.GetOrBuild(dirs, index.CachePath(), false)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error building index: %v\n", err)
		os.Exit(1)
	}

	// Run keyword matching (Layer 1 — always runs)
	engine := matcher.NewEngine(cfg)
	results := engine.Query(idx, query)

	// Apply AI reranking if configured (Layer 2 — optional)
	p := provider.GetWithFallback(cfg)
	if p.Name() != "local" {
		reranked, err := p.Rerank(results, query)
		if err != nil {
			if verbose {
				fmt.Fprintf(os.Stderr, "AI rerank failed (%s): %v, using keyword results\n", p.Name(), err)
			}
			// Graceful fallback: keep keyword results
		} else {
			results = reranked
		}
	}

	elapsed := time.Since(start)

	if verbose {
		fmt.Fprintf(os.Stderr, "Index: %d skills from %d dirs (cached: %s)\n",
			len(idx.Entries), len(idx.SkillDirs), idx.BuiltAt)
		fmt.Fprintf(os.Stderr, "Query: %q → %d results in %.1fms (provider: %s)\n",
			query, len(results), float64(elapsed.Microseconds())/1000.0, p.Name())
	}

	// Output JSON
	output := QueryOutput{
		Query:     query,
		ElapsedMs: float64(elapsed.Microseconds()) / 1000.0,
		Total:     len(results),
		Provider:  p.Name(),
		Results:   results,
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(output); err != nil {
		fmt.Fprintf(os.Stderr, "Error encoding output: %v\n", err)
		os.Exit(1)
	}
}

// cmdIndex implements the "index" subcommand.
func cmdIndex() {
	force := false
	for _, arg := range os.Args[2:] {
		if arg == "--force" || arg == "-f" {
			force = true
		}
	}

	cfg, err := config.Load(config.ConfigPath())
	if err != nil {
		cfg = config.DefaultConfig()
	}

	start := time.Now()
	dirs := cfg.ExpandedSkillDirs()

	idx, err := index.GetOrBuild(dirs, index.CachePath(), force)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error building index: %v\n", err)
		os.Exit(1)
	}

	elapsed := time.Since(start)
	fmt.Printf("Indexed %d skills from %d directories in %.1fms\n",
		len(idx.Entries), len(dirs), float64(elapsed.Microseconds())/1000.0)
	fmt.Printf("Cache saved to: %s\n", index.CachePath())
}

// cmdStatus implements the "status" subcommand.
func cmdStatus() {
	cfg, err := config.Load(config.ConfigPath())
	if err != nil {
		fmt.Fprintf(os.Stderr, "Config error: %v\n", err)
		cfg = config.DefaultConfig()
	}

	fmt.Println("=== SKRT Status ===")
	fmt.Printf("Config: %s\n", config.ConfigPath())
	fmt.Printf("Cache:  %s\n", index.CachePath())
	fmt.Printf("Provider: %s\n", cfg.Provider)

	fmt.Println("\nSkill Directories:")
	dirs := cfg.ExpandedSkillDirs()
	for _, d := range dirs {
		exists := "✓"
		if _, err := os.Stat(d); os.IsNotExist(err) {
			exists = "✗"
		}
		fmt.Printf("  %s %s\n", exists, d)
	}

	fmt.Println("\nPinned Skills:")
	if len(cfg.Pinned) == 0 {
		fmt.Println("  (none)")
	} else {
		for _, p := range cfg.Pinned {
			w := cfg.GetWeight(p)
			if w > 0 {
				fmt.Printf("  📌 %s (weight: +%d)\n", p, w)
			} else {
				fmt.Printf("  📌 %s\n", p)
			}
		}
	}

	// Try loading cached index
	cached, err := index.LoadCache(index.CachePath())
	if err != nil || cached == nil {
		fmt.Println("\nIndex: not built (run: skrt index)")
	} else {
		fmt.Printf("\nIndex:\n")
		fmt.Printf("  Skills:  %d\n", len(cached.Entries))
		fmt.Printf("  Built:   %s\n", cached.BuiltAt)
		fmt.Printf("  Version: %d\n", cached.Version)
	}

	fmt.Printf("\nSettings:\n")
	fmt.Printf("  top_n:     %d\n", cfg.TopN)
	fmt.Printf("  min_score: %d\n", cfg.MinScore)
}

// cmdPin implements the "pin" subcommand.
func cmdPin() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "Usage: skrt pin <add|remove|list> [name]")
		os.Exit(1)
	}

	action := os.Args[2]
	cfgPath := config.ConfigPath()
	cfg, err := config.Load(cfgPath)
	if err != nil {
		cfg = config.DefaultConfig()
	}

	switch action {
	case "add":
		if len(os.Args) < 4 {
			fmt.Fprintln(os.Stderr, "Usage: skrt pin add <skill-name>")
			os.Exit(1)
		}
		name := os.Args[3]
		if cfg.AddPin(name) {
			if err := config.Save(cfgPath, cfg); err != nil {
				fmt.Fprintf(os.Stderr, "Error saving config: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("📌 Pinned: %s\n", name)
		} else {
			fmt.Printf("Already pinned: %s\n", name)
		}

	case "remove", "rm":
		if len(os.Args) < 4 {
			fmt.Fprintln(os.Stderr, "Usage: skrt pin remove <skill-name>")
			os.Exit(1)
		}
		name := os.Args[3]
		if cfg.RemovePin(name) {
			if err := config.Save(cfgPath, cfg); err != nil {
				fmt.Fprintf(os.Stderr, "Error saving config: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("Unpinned: %s\n", name)
		} else {
			fmt.Printf("Not pinned: %s\n", name)
		}

	case "list", "ls":
		if len(cfg.Pinned) == 0 {
			fmt.Println("No pinned skills.")
		} else {
			for _, p := range cfg.Pinned {
				fmt.Printf("  📌 %s\n", p)
			}
		}

	default:
		fmt.Fprintf(os.Stderr, "Unknown pin action: %s\n", action)
		os.Exit(1)
	}
}

// cmdDir implements the "dir" subcommand.
func cmdDir() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "Usage: skrt dir <add|remove|list> [path]")
		os.Exit(1)
	}

	action := os.Args[2]
	cfgPath := config.ConfigPath()
	cfg, err := config.Load(cfgPath)
	if err != nil {
		cfg = config.DefaultConfig()
	}

	switch action {
	case "add":
		if len(os.Args) < 4 {
			fmt.Fprintln(os.Stderr, "Usage: skrt dir add <path>")
			os.Exit(1)
		}
		path := os.Args[3]
		for _, d := range cfg.SkillDirs {
			if d == path {
				fmt.Printf("Already registered: %s\n", path)
				return
			}
		}
		cfg.SkillDirs = append(cfg.SkillDirs, path)
		if err := config.Save(cfgPath, cfg); err != nil {
			fmt.Fprintf(os.Stderr, "Error saving config: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Added: %s\n", path)

	case "remove", "rm":
		if len(os.Args) < 4 {
			fmt.Fprintln(os.Stderr, "Usage: skrt dir remove <path>")
			os.Exit(1)
		}
		path := os.Args[3]
		found := false
		var newDirs []string
		for _, d := range cfg.SkillDirs {
			if d == path {
				found = true
			} else {
				newDirs = append(newDirs, d)
			}
		}
		if found {
			cfg.SkillDirs = newDirs
			if err := config.Save(cfgPath, cfg); err != nil {
				fmt.Fprintf(os.Stderr, "Error saving config: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("Removed: %s\n", path)
		} else {
			fmt.Printf("Not found: %s\n", path)
		}

	case "list", "ls":
		dirs := cfg.ExpandedSkillDirs()
		for i, d := range dirs {
			exists := "✓"
			if _, err := os.Stat(d); os.IsNotExist(err) {
				exists = "✗"
			}
			raw := cfg.SkillDirs[i]
			if raw != d {
				fmt.Printf("  %s %s → %s\n", exists, raw, d)
			} else {
				fmt.Printf("  %s %s\n", exists, d)
			}
		}

	default:
		fmt.Fprintf(os.Stderr, "Unknown dir action: %s\n", action)
		os.Exit(1)
	}
}

// cmdVersion implements the "version" subcommand.
func cmdVersion() {
	fmt.Printf("skrt %s (%s, %s)\n", version, gitCommit, buildTime)
}

// cmdProvider implements the "provider" subcommand.
// Manages AI provider configuration for enhanced semantic reranking.
func cmdProvider() {
	if len(os.Args) < 3 {
		cmdProviderStatus()
		return
	}

	action := os.Args[2]
	cfgPath := config.ConfigPath()
	cfg, err := config.Load(cfgPath)
	if err != nil {
		cfg = config.DefaultConfig()
	}

	switch action {
	case "status", "st":
		cmdProviderStatus()

	case "set":
		if len(os.Args) < 4 {
			fmt.Fprintln(os.Stderr, "Usage: skrt provider set <local|api>")
			os.Exit(1)
		}
		name := os.Args[3]
		if name != "local" && name != "api" {
			fmt.Fprintf(os.Stderr, "Unknown provider: %s (supported: local, api)\n", name)
			os.Exit(1)
		}
		cfg.Provider = name
		if err := config.Save(cfgPath, cfg); err != nil {
			fmt.Fprintf(os.Stderr, "Error saving config: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("✅ Provider set to: %s\n", name)

	case "setup":
		cmdProviderSetup()

	case "models":
		fmt.Print(`📋 Supported Embedding Models

Gemini (Recommended):
  gemini-embedding-001          768 dims   Fast, text-only        ⭐ Best for SKRT
  gemini-embedding-2-preview    3072 dims  Multimodal, highest quality
  text-embedding-004            768 dims   Legacy, stable

OpenAI-compatible:
  text-embedding-3-small        1536 dims  Fast, cheap
  text-embedding-3-large        3072 dims  Highest quality
  (any Ollama/LM Studio/vLLM model with /embeddings endpoint)

💡 Recommendation for SKRT:
  → gemini-embedding-001 — best price/performance, fast, sufficient for skill routing
  → gemini-embedding-2-preview — if you need the highest matching accuracy
`)

	default:
		fmt.Fprintf(os.Stderr, "Unknown provider action: %s\n", action)
		fmt.Fprintln(os.Stderr, "Usage: skrt provider <status|set|setup|models>")
		os.Exit(1)
	}
}

// cmdProviderStatus shows the current provider configuration.
func cmdProviderStatus() {
	cfg, err := config.Load(config.ConfigPath())
	if err != nil {
		cfg = config.DefaultConfig()
	}

	fmt.Println("=== SKRT Provider Status ===")
	fmt.Printf("Active Provider: %s\n", cfg.Provider)

	if cfg.Provider == "local" {
		fmt.Println("\n  Mode: Keyword matching only (zero dependencies)")
		fmt.Println("  Speed: ~3ms matching + ~50ms I/O overhead")
		fmt.Println("\n  To enable AI reranking: skrt provider setup")
		return
	}

	pc := cfg.GetProviderConfig("api")
	fc := cfg.GetFusion()

	fmt.Printf("\n  Endpoint:   %s\n", pc.Endpoint)
	fmt.Printf("  Model:      %s\n", pc.Model)
	fmt.Printf("  API Key:    $%s", pc.APIKeyEnv)

	apiKey, source := credentials.Resolve(pc.APIKeyEnv)
	if apiKey != "" {
		masked := apiKey[:4] + "..." + apiKey[len(apiKey)-4:]
		fmt.Printf(" ✅ (%s, %s)\n", masked, source)
	} else {
		fmt.Printf(" ❌ (not set — run: skrt provider setup)\n")
	}

	fmt.Printf("\n  Fusion:\n")
	fmt.Printf("    Keyword weight: %.1f\n", fc.KeywordWeight)
	fmt.Printf("    AI weight:      %.1f\n", fc.AIWeight)
	fmt.Printf("    Timeout:        %dms\n", fc.TimeoutMs)
}

// cmdProviderSetup creates a Gemini API configuration with secure key storage.
func cmdProviderSetup() {
	cfgPath := config.ConfigPath()
	cfg, err := config.Load(cfgPath)
	if err != nil {
		cfg = config.DefaultConfig()
	}

	// Parse flags
	model := "gemini-embedding-001"
	apiKeyEnv := "GEMINI_API_KEY"
	endpoint := "https://generativelanguage.googleapis.com/v1beta"
	kwWeight := 0.6
	aiWeight := 0.4
	apiKeyInput := ""

	for i := 3; i < len(os.Args); i++ {
		switch os.Args[i] {
		case "--model", "-m":
			if i+1 < len(os.Args) {
				i++
				model = os.Args[i]
			}
		case "--env":
			if i+1 < len(os.Args) {
				i++
				apiKeyEnv = os.Args[i]
			}
		case "--endpoint":
			if i+1 < len(os.Args) {
				i++
				endpoint = os.Args[i]
			}
		case "--key":
			if i+1 < len(os.Args) {
				i++
				apiKeyInput = os.Args[i]
			}
		case "--kw-weight":
			if i+1 < len(os.Args) {
				i++
				fmt.Sscanf(os.Args[i], "%f", &kwWeight)
			}
		case "--ai-weight":
			if i+1 < len(os.Args) {
				i++
				fmt.Sscanf(os.Args[i], "%f", &aiWeight)
			}
		}
	}

	// Check if key already exists
	existingKey, source := credentials.Resolve(apiKeyEnv)

	// If no key provided via flag and no existing key, prompt for input
	if apiKeyInput == "" && existingKey == "" {
		fmt.Printf("\n🔑 Enter your Gemini API key (get one at https://aistudio.google.com/apikey):\n")
		fmt.Printf("   Paste key: ")
		reader := bufio.NewReader(os.Stdin)
		line, _ := reader.ReadString('\n')
		apiKeyInput = strings.TrimSpace(line)

		if apiKeyInput == "" {
			fmt.Fprintln(os.Stderr, "\n❌ No API key provided. Aborting.")
			os.Exit(1)
		}
	}

	// Store the key securely
	if apiKeyInput != "" {
		if err := credentials.Store(apiKeyEnv, apiKeyInput); err != nil {
			fmt.Fprintf(os.Stderr, "Error storing key: %v\n", err)
			os.Exit(1)
		}
		masked := apiKeyInput[:4] + "..." + apiKeyInput[len(apiKeyInput)-4:]
		fmt.Printf("\n🔒 API key stored securely: %s\n", masked)
		fmt.Printf("   Location: ~/.skrt/credentials (permissions: 0600)\n")
	} else if existingKey != "" {
		masked := existingKey[:4] + "..." + existingKey[len(existingKey)-4:]
		fmt.Printf("\n🔑 Using existing key: %s (source: %s)\n", masked, source)
	}

	// Write config (no key in config, only env var name)
	if cfg.Providers == nil {
		cfg.Providers = make(map[string]config.ProviderConfig)
	}
	cfg.Providers["api"] = config.ProviderConfig{
		Endpoint:  endpoint,
		APIKeyEnv: apiKeyEnv,
		Model:     model,
	}
	cfg.Provider = "api"
	cfg.Fusion = &config.FusionConfig{
		KeywordWeight: kwWeight,
		AIWeight:      aiWeight,
		TimeoutMs:     10000,
	}

	if err := config.Save(cfgPath, cfg); err != nil {
		fmt.Fprintf(os.Stderr, "Error saving config: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("\n✅ API provider configured!")
	fmt.Printf("  Model:    %s\n", model)
	fmt.Printf("  Endpoint: %s\n", endpoint)
	fmt.Printf("  Key Ref:  $%s\n", apiKeyEnv)
	fmt.Printf("  Fusion:   keyword=%.1f, ai=%.1f\n", kwWeight, aiWeight)
	fmt.Println("\n  Try: skrt query \"PDF merge\" --verbose")
}

// printUsage prints the usage information.
func printUsage() {
	fmt.Print(`⚡ SKRT — Skill Router for AI Agents

Blazing-fast skill router for AI coding agents.
Works with Antigravity, Claude Code, Codex, Cursor, Gemini CLI,
and any agent that uses SKILL.md files.

Usage:
  skrt query <terms>            Search for matching skills (alias: q)
  skrt <terms>                  Shorthand for 'query' (auto-detected)
  skrt index [--force]          Rebuild the skill index (alias: idx)
  skrt status                   Show index and config status (alias: st)
  skrt pin add <name>           Pin a skill for priority boosting
  skrt pin remove <name>        Unpin a skill
  skrt pin list                 List pinned skills
  skrt dir add <path>           Add a skill search directory
  skrt dir remove <path>        Remove a skill search directory
  skrt dir list                 List skill search directories
  skrt provider status          Show AI provider configuration
  skrt provider set <name>      Switch provider: local, api
  skrt provider setup           Configure Gemini API provider
  skrt provider models          List supported embedding models
  skrt version                  Show version information

Query Options:
  --verbose, -v          Show debug info on stderr
  --top N                Override max results (default: 5)
  --provider, -p NAME    Use AI provider: local (default), api

Provider Setup:
  skrt provider setup --model gemini-embedding-001
  skrt provider setup --model gemini-embedding-2-preview
  skrt provider setup --endpoint https://api.openai.com/v1 --env OPENAI_API_KEY

Examples:
  skrt query "NotebookLM search"
  skrt "PDF merge"
  skrt q "molecular docking" --verbose --top 10
  skrt q "protein structure" --provider api
  skrt pin add brainstorming
  skrt dir add ~/my-custom-skills

Config: ~/.skrt/config.json
Cache:  ~/.skrt/index.json
`)
}

