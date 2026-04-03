// Package unicode provides shared Unicode utility functions for CJK text processing.
// These functions are used by both the indexer and the matching engine.
package unicode

// IsCJK returns true if the rune is a CJK Unified Ideograph.
// Covers the most common CJK blocks used in Chinese, Japanese, and Korean.
func IsCJK(r rune) bool {
	return (r >= 0x4E00 && r <= 0x9FFF) || // CJK Unified Ideographs
		(r >= 0x3400 && r <= 0x4DBF) || // CJK Extension A
		(r >= 0xF900 && r <= 0xFAFF) // CJK Compatibility Ideographs
}

// IsAlphaNumCJK returns true for letters, digits, and CJK characters.
func IsAlphaNumCJK(r rune) bool {
	if r >= 'a' && r <= 'z' {
		return true
	}
	if r >= 'A' && r <= 'Z' {
		return true
	}
	if r >= '0' && r <= '9' {
		return true
	}
	return IsCJK(r)
}
