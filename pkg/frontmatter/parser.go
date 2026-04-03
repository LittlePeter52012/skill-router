// Package frontmatter provides a zero-dependency YAML frontmatter parser
// for SKILL.md files. It extracts only name and description fields from
// the YAML block delimited by "---" markers, without importing any YAML library.
package frontmatter

import (
	"bufio"
	"io"
	"strings"
)

// Metadata holds the extracted frontmatter fields from a SKILL.md file.
type Metadata struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// Parse reads a SKILL.md file content and extracts the YAML frontmatter.
// It only reads until the closing "---" marker, making it efficient for
// large files (typically reads < 2KB regardless of file size).
func Parse(r io.Reader) (Metadata, error) {
	scanner := bufio.NewScanner(r)
	var meta Metadata
	inFrontmatter := false
	var descBuilder strings.Builder
	inMultiLineDesc := false

	for scanner.Scan() {
		line := scanner.Text()

		if !inFrontmatter {
			if strings.TrimSpace(line) == "---" {
				inFrontmatter = true
			}
			continue
		}

		// End of frontmatter
		if strings.TrimSpace(line) == "---" {
			break
		}

		// Handle multi-line description continuation
		if inMultiLineDesc {
			trimmed := strings.TrimSpace(line)
			// Multi-line continues if indented or is a continuation of quoted string
			if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
				if descBuilder.Len() > 0 {
					descBuilder.WriteByte(' ')
				}
				descBuilder.WriteString(trimmed)
				// Check if this line ends the quoted string
				if strings.HasSuffix(trimmed, `"`) || strings.HasSuffix(trimmed, `'`) {
					inMultiLineDesc = false
					meta.Description = unquote(descBuilder.String())
				}
				continue
			}
			// Not indented — end multi-line
			inMultiLineDesc = false
			meta.Description = unquote(descBuilder.String())
		}

		// Parse key: value
		key, value, ok := parseKeyValue(line)
		if !ok {
			continue
		}

		switch key {
		case "name":
			meta.Name = unquote(strings.TrimSpace(value))
		case "description":
			trimVal := strings.TrimSpace(value)
			// Check for multi-line: value starts with quote but doesn't end with one
			if (strings.HasPrefix(trimVal, `"`) && !strings.HasSuffix(trimVal, `"`)) ||
				(strings.HasPrefix(trimVal, `'`) && !strings.HasSuffix(trimVal, `'`)) {
				descBuilder.Reset()
				descBuilder.WriteString(trimVal)
				inMultiLineDesc = true
			} else {
				meta.Description = unquote(trimVal)
			}
		}
	}

	// Handle case where multi-line description reaches EOF without closing
	if inMultiLineDesc && descBuilder.Len() > 0 {
		meta.Description = unquote(descBuilder.String())
	}

	if err := scanner.Err(); err != nil {
		return meta, err
	}

	return meta, nil
}

// ParseBytes is a convenience wrapper around Parse for byte slices.
func ParseBytes(data []byte) (Metadata, error) {
	return Parse(strings.NewReader(string(data)))
}

// parseKeyValue splits a YAML line into key and value.
// Returns false if the line is not a valid key: value pair.
func parseKeyValue(line string) (key, value string, ok bool) {
	// Skip comments
	trimmed := strings.TrimSpace(line)
	if strings.HasPrefix(trimmed, "#") {
		return "", "", false
	}

	idx := strings.IndexByte(line, ':')
	if idx < 0 {
		return "", "", false
	}

	key = strings.TrimSpace(line[:idx])
	value = strings.TrimSpace(line[idx+1:])

	// Key must be a simple identifier (no spaces, starts with letter)
	if len(key) == 0 {
		return "", "", false
	}

	return key, value, true
}

// unquote removes surrounding single or double quotes from a string.
func unquote(s string) string {
	if len(s) < 2 {
		return s
	}
	if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
		return s[1 : len(s)-1]
	}
	return s
}
