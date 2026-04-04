// Package index manages caching of the skill index for sub-millisecond lookups.
// Cache is stored as JSON at ~/.skrt/index.json and validated via a two-tier
// strategy: fast directory mtime check (~0.1ms) then full SHA256 checksum
// only when directories have changed (new skill added, file modified).
package index

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

const (
	cacheVersion = 2 // Bumped for new two-tier validation
	cacheDir     = ".skrt"
	cacheFile    = "index.json"
)

// CachePath returns the default path for the index cache file.
func CachePath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, cacheDir, cacheFile)
}

// LoadCache reads the cached index from disk. Returns nil if cache
// doesn't exist or is invalid.
func LoadCache(path string) (*Index, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read cache: %w", err)
	}

	var idx Index
	if err := json.Unmarshal(data, &idx); err != nil {
		return nil, fmt.Errorf("parse cache: %w", err)
	}

	if idx.Version != cacheVersion {
		return nil, nil // Version mismatch, rebuild
	}

	return &idx, nil
}

// SaveCache writes the index to the cache file, creating directories as needed.
func SaveCache(path string, idx *Index) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create cache dir: %w", err)
	}

	data, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal cache: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write cache: %w", err)
	}

	return nil
}

// IsCacheValid checks if the cached index matches the current state of
// skill directories using a two-tier validation strategy:
//
//	Tier 1 (Fast, ~0.1ms): Compare the cache file's mtime against each
//	       skill directory's mtime. If cache is newer than ALL dirs, skip
//	       the expensive checksum walk.
//
//	Tier 2 (Thorough, ~30-50ms): Full directory walk + SHA256 checksum.
//	       Only runs when a directory has been modified since the last cache.
//
// This means: if no SKILL.md was added/removed/modified since the last cache,
// validation completes in <1ms instead of 50ms.
func IsCacheValid(cached *Index, dirs []string, cachePath string, ignoreDirNames []string) bool {
	if cached == nil {
		return false
	}

	// Quick check: same directories?
	if len(cached.SkillDirs) != len(dirs) {
		return false
	}
	dirSet := make(map[string]bool)
	for _, d := range cached.SkillDirs {
		dirSet[d] = true
	}
	for _, d := range dirs {
		if !dirSet[d] {
			return false
		}
	}

	if len(cached.IgnoreDirNames) != len(ignoreDirNames) {
		return false
	}
	ignoreSet := make(map[string]bool, len(cached.IgnoreDirNames))
	for _, name := range cached.IgnoreDirNames {
		ignoreSet[name] = true
	}
	for _, name := range ignoreDirNames {
		if !ignoreSet[name] {
			return false
		}
	}

	// === Tier 1: Fast mtime check ===
	// If the cache file is newer than all skill directories (and their
	// immediate children), no files have been added/removed/modified.
	cacheInfo, err := os.Stat(cachePath)
	if err != nil {
		return false // Cache file doesn't exist or inaccessible
	}
	cacheMtime := cacheInfo.ModTime()

	allFresh := true
	for _, dir := range dirs {
		// Check the directory itself (adding/deleting a child changes dir mtime)
		dirInfo, err := os.Stat(dir)
		if err != nil {
			allFresh = false
			break
		}
		if dirInfo.ModTime().After(cacheMtime) {
			allFresh = false
			break
		}

		// Check immediate subdirectories (where SKILL.md files live)
		entries, err := os.ReadDir(dir)
		if err != nil {
			allFresh = false
			break
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			subDir := filepath.Join(dir, entry.Name())
			subInfo, err := os.Stat(subDir)
			if err != nil {
				continue
			}
			if subInfo.ModTime().After(cacheMtime) {
				allFresh = false
				break
			}
		}
		if !allFresh {
			break
		}
	}

	if allFresh {
		return true // Fast path: no directory changes detected
	}

	// === Tier 2: Full checksum verification ===
	current := computeChecksum(dirs, ignoreDirNames)
	return cached.Checksum == current
}

// computeChecksum generates a deterministic SHA256 hash based on all SKILL.md
// paths and their modification times. This ensures cache invalidation when
// files are added, removed, or modified.
//
// The paths are sorted before hashing to ensure deterministic output regardless
// of filesystem walk order.
func computeChecksum(dirs []string, ignoreDirNames []string) string {
	var items []string
	ignoreSet := make(map[string]struct{}, len(ignoreDirNames))
	for _, name := range ignoreDirNames {
		ignoreSet[name] = struct{}{}
	}

	for _, dir := range dirs {
		_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				if _, skip := ignoreSet[d.Name()]; skip {
					return filepath.SkipDir
				}
				return nil
			}
			if d.Name() == "SKILL.md" {
				info, err := d.Info()
				if err != nil {
					return nil
				}
				items = append(items, fmt.Sprintf("%s:%d", path, info.ModTime().Unix()))
			}
			return nil
		})
	}

	// Sort for deterministic ordering across different OS/filesystem behaviors
	sort.Strings(items)

	h := sha256.New()
	fmt.Fprintf(h, "v%d:", cacheVersion)
	for _, item := range items {
		fmt.Fprintf(h, "%s|", item)
	}
	return fmt.Sprintf("%x", h.Sum(nil))[:16]
}

// GetOrBuild loads the index from cache if valid, otherwise rebuilds it.
// This is the main entry point for obtaining the skill index.
func GetOrBuild(dirs []string, cachePath string, forceRebuild bool, ignoreDirNames []string) (*Index, error) {
	if !forceRebuild {
		cached, err := LoadCache(cachePath)
		if err == nil && IsCacheValid(cached, dirs, cachePath, ignoreDirNames) {
			return cached, nil
		}
	}

	// Rebuild
	idx, err := Build(dirs, ignoreDirNames)
	if err != nil {
		return nil, err
	}

	// Save to cache (non-fatal if fails)
	_ = SaveCache(cachePath, idx)

	return idx, nil
}
