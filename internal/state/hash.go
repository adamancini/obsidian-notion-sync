package state

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"
)

var (
	// frontmatterDelimiter matches YAML frontmatter delimiters.
	frontmatterDelimiter = []byte("---")
	// trailingWhitespace matches trailing whitespace on each line.
	// (?m) enables multiline mode so $ matches end of each line.
	trailingWhitespace = regexp.MustCompile(`(?m)[ \t]+$`)
	// multipleBlankLines matches 2+ consecutive blank lines.
	multipleBlankLines = regexp.MustCompile(`\n{3,}`)
)

// ContentHashes holds both content and frontmatter hashes.
type ContentHashes struct {
	// ContentHash is the hash of the normalized body content (excluding frontmatter).
	ContentHash string
	// FrontmatterHash is the hash of the normalized frontmatter YAML.
	FrontmatterHash string
	// FullHash is the hash of the entire normalized content.
	FullHash string
}

// HashContent computes normalized content hashes for a markdown file.
// Normalization includes:
// - Removing trailing whitespace from each line
// - Normalizing line endings to \n
// - Collapsing multiple blank lines to at most 2
// - Trimming leading/trailing whitespace from the file
func HashContent(content []byte) ContentHashes {
	// Split frontmatter and body.
	frontmatter, body := splitFrontmatter(content)

	// Normalize both parts.
	normalizedFM := normalizeContent(frontmatter)
	normalizedBody := normalizeContent(body)

	// Compute hashes.
	var hashes ContentHashes

	if len(normalizedBody) > 0 {
		hashes.ContentHash = computeHash(normalizedBody)
	}

	if len(normalizedFM) > 0 {
		hashes.FrontmatterHash = computeHash(normalizedFM)
	}

	// Full hash combines both (or just body if no frontmatter).
	var fullContent []byte
	if len(normalizedFM) > 0 {
		fullContent = append(normalizedFM, '\n')
		fullContent = append(fullContent, normalizedBody...)
	} else {
		fullContent = normalizedBody
	}
	if len(fullContent) > 0 {
		hashes.FullHash = computeHash(fullContent)
	}

	return hashes
}

// HashContentRaw computes a hash of content without normalization.
// Use this when exact byte-for-byte comparison is needed.
func HashContentRaw(content []byte) string {
	return computeHash(content)
}

// splitFrontmatter separates YAML frontmatter from body content.
// Returns (frontmatter, body) where frontmatter excludes the --- delimiters.
func splitFrontmatter(content []byte) ([]byte, []byte) {
	// Check for frontmatter delimiter at start.
	if !bytes.HasPrefix(content, frontmatterDelimiter) {
		return nil, content
	}

	// Must be followed by newline.
	if len(content) <= 3 || content[3] != '\n' {
		return nil, content
	}

	// Find closing delimiter.
	rest := content[4:] // Skip "---\n"
	idx := bytes.Index(rest, []byte("\n---\n"))
	if idx == -1 {
		// Check for frontmatter at end of file (no trailing newline after ---).
		idx = bytes.Index(rest, []byte("\n---"))
		if idx == -1 || idx+4 != len(rest) {
			return nil, content
		}
		// Frontmatter goes to end.
		return rest[:idx], nil
	}

	frontmatter := rest[:idx]
	body := rest[idx+5:] // Skip "\n---\n"

	return frontmatter, body
}

// normalizeContent applies normalization rules to content.
func normalizeContent(content []byte) []byte {
	if len(content) == 0 {
		return nil
	}

	// Convert to string for easier processing.
	s := string(content)

	// Normalize line endings to \n.
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")

	// Remove trailing whitespace from each line.
	s = trailingWhitespace.ReplaceAllString(s, "")

	// Collapse multiple blank lines (3+ newlines -> 2 newlines).
	s = multipleBlankLines.ReplaceAllString(s, "\n\n")

	// Trim leading and trailing whitespace from the entire content.
	s = strings.TrimSpace(s)

	if len(s) == 0 {
		return nil
	}

	return []byte(s)
}

// computeHash computes SHA-256 hash and returns hex string.
func computeHash(content []byte) string {
	hash := sha256.Sum256(content)
	return hex.EncodeToString(hash[:])
}

// HasContentChanged checks if content has changed by comparing hashes.
// Returns true if either content or frontmatter has changed.
func HasContentChanged(oldHashes, newHashes ContentHashes) bool {
	return oldHashes.ContentHash != newHashes.ContentHash ||
		oldHashes.FrontmatterHash != newHashes.FrontmatterHash
}

// HasBodyChanged checks if only the body content has changed.
// Useful for detecting content-only changes vs metadata-only changes.
func HasBodyChanged(oldHashes, newHashes ContentHashes) bool {
	return oldHashes.ContentHash != newHashes.ContentHash
}

// HasFrontmatterChanged checks if only the frontmatter has changed.
// Useful for detecting metadata-only changes that might only need property updates.
func HasFrontmatterChanged(oldHashes, newHashes ContentHashes) bool {
	return oldHashes.FrontmatterHash != newHashes.FrontmatterHash
}

// HashesFromState creates ContentHashes from a SyncState.
// Returns empty ContentHashes if state is nil.
func HashesFromState(state *SyncState) ContentHashes {
	if state == nil {
		return ContentHashes{}
	}
	return ContentHashes{
		ContentHash:     state.ContentHash,
		FrontmatterHash: state.FrontmatterHash,
		FullHash:        "", // Not stored in state
	}
}
