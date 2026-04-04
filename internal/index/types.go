// Package index handles scanning SKILL.md files from configured directories,
// building an in-memory index of all skills, and caching the index to disk
// for sub-millisecond subsequent loads.
package index

// SkillEntry represents a single indexed skill with its metadata and search tokens.
type SkillEntry struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Path        string   `json:"path"`     // Absolute path to SKILL.md
	Dir         string   `json:"dir"`      // Parent directory name (skill ID)
	Tokens      []string `json:"tokens"`   // Pre-computed search tokens from name + description
	ModTime     int64    `json:"mod_time"` // File modification time (Unix)
}

// Index holds the complete skill index with metadata for cache validation.
type Index struct {
	Version        int          `json:"version"`
	BuiltAt        string       `json:"built_at"`
	Checksum       string       `json:"checksum"` // Hash of all file paths + mod times
	SkillDirs      []string     `json:"skill_dirs"`
	IgnoreDirNames []string     `json:"ignore_dir_names,omitempty"`
	Entries        []SkillEntry `json:"entries"`
}

// scanResult is used internally for concurrent file scanning.
type scanResult struct {
	entry SkillEntry
	err   error
}
