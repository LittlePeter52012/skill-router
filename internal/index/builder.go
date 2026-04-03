// Package index builds and manages the skill index, scanning SKILL.md files
// from configured directories and caching the results for fast lookup.
package index

import (
	"crypto/sha256"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/skrt-dev/skill-router/internal/unicode"
	"github.com/skrt-dev/skill-router/pkg/frontmatter"
)

// Build scans all configured skill directories concurrently and builds
// a complete index of available skills.
func Build(dirs []string) (*Index, error) {
	// Discover all SKILL.md files
	var paths []string
	for _, dir := range dirs {
		expanded := expandHome(dir)
		_ = filepath.WalkDir(expanded, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil // Skip inaccessible paths
			}
			if d.Name() == "SKILL.md" && !d.IsDir() {
				paths = append(paths, path)
			}
			return nil
		})
	}

	if len(paths) == 0 {
		return &Index{
			Version:   cacheVersion,
			BuiltAt:   time.Now().UTC().Format(time.RFC3339),
			SkillDirs: dirs,
			Entries:   []SkillEntry{},
		}, nil
	}

	// Concurrent scanning with bounded goroutines
	const maxWorkers = 32
	resultCh := make(chan scanResult, len(paths))
	sem := make(chan struct{}, maxWorkers)
	var wg sync.WaitGroup

	for _, p := range paths {
		wg.Add(1)
		go func(path string) {
			defer wg.Done()
			sem <- struct{}{}        // Acquire
			defer func() { <-sem }() // Release

			entry, err := scanSkillFile(path)
			resultCh <- scanResult{entry: entry, err: err}
		}(p)
	}

	// Close channel when all goroutines finish
	go func() {
		wg.Wait()
		close(resultCh)
	}()

	// Collect results
	var entries []SkillEntry
	for result := range resultCh {
		if result.err != nil {
			continue // Skip files that fail to parse
		}
		if result.entry.Name == "" {
			continue // Skip entries without a name
		}
		entries = append(entries, result.entry)
	}

	// Compute checksum
	checksum := buildChecksum(entries)

	return &Index{
		Version:   cacheVersion,
		BuiltAt:   time.Now().UTC().Format(time.RFC3339),
		Checksum:  checksum,
		SkillDirs: dirs,
		Entries:   entries,
	}, nil
}

// scanSkillFile reads a single SKILL.md and extracts metadata.
func scanSkillFile(path string) (SkillEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return SkillEntry{}, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	// Read only the first 4KB for frontmatter (fast even for huge files)
	buf := make([]byte, 4096)
	n, _ := f.Read(buf)

	meta, err := frontmatter.ParseBytes(buf[:n])
	if err != nil {
		return SkillEntry{}, fmt.Errorf("parse %s: %w", path, err)
	}

	// Get file info for mod time
	info, err := os.Stat(path)
	if err != nil {
		return SkillEntry{}, fmt.Errorf("stat %s: %w", path, err)
	}

	// Extract the skill directory name (parent of SKILL.md)
	dir := filepath.Base(filepath.Dir(path))

	// Use directory name as skill name if frontmatter name is empty
	name := meta.Name
	if name == "" {
		name = dir
	}

	// Generate search tokens
	tokens := tokenize(name, meta.Description)

	return SkillEntry{
		Name:        name,
		Description: meta.Description,
		Path:        path,
		Dir:         dir,
		Tokens:      tokens,
		ModTime:     info.ModTime().Unix(),
	}, nil
}

// tokenize generates search tokens from name and description.
// It performs lowercasing, splitting by common delimiters, and
// extracts CJK bigrams for Chinese/Japanese/Korean matching.
func tokenize(name, description string) []string {
	tokenSet := make(map[string]bool)

	// Add the full name and its lowercase variant
	lower := strings.ToLower(name)
	tokenSet[lower] = true

	// Split name by hyphens and underscores
	for _, part := range strings.FieldsFunc(lower, func(r rune) bool {
		return r == '-' || r == '_' || r == ' '
	}) {
		if len(part) > 1 {
			tokenSet[part] = true
		}
	}

	// Tokenize description
	combined := strings.ToLower(name + " " + description)
	for _, word := range strings.FieldsFunc(combined, func(r rune) bool {
		return !unicode.IsAlphaNumCJK(r)
	}) {
		if len(word) > 1 || unicode.IsCJK(rune(word[0])) {
			tokenSet[word] = true
		}
	}

	// Extract CJK bigrams for better Chinese matching
	runes := []rune(combined)
	for i := 0; i < len(runes)-1; i++ {
		if unicode.IsCJK(runes[i]) && unicode.IsCJK(runes[i+1]) {
			tokenSet[string(runes[i:i+2])] = true
		}
	}

	// Convert to slice
	tokens := make([]string, 0, len(tokenSet))
	for t := range tokenSet {
		tokens = append(tokens, t)
	}
	return tokens
}

// buildChecksum creates a deterministic hash of all entries for cache validation.
func buildChecksum(entries []SkillEntry) string {
	h := sha256.New()
	for _, e := range entries {
		fmt.Fprintf(h, "%s:%d|", e.Path, e.ModTime)
	}
	return fmt.Sprintf("%x", h.Sum(nil))[:16] // Short hash is sufficient
}

// expandHome expands ~ to the user's home directory.
func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, path[2:])
	}
	return path
}
