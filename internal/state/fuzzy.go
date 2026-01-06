package state

import (
	"path/filepath"
	"strings"
	"unicode"
)

// MatchScore represents the quality of a fuzzy match.
type MatchScore int

const (
	MatchNone           MatchScore = 0
	MatchFuzzy          MatchScore = 1 // Within Levenshtein threshold
	MatchPrefix         MatchScore = 2 // Target starts with query
	MatchCaseInsensitive MatchScore = 3 // Exact match ignoring case
	MatchExact          MatchScore = 4 // Exact match
)

// MatchResult represents a potential match with its score and path.
type MatchResult struct {
	Path     string
	PageID   string
	Name     string
	Score    MatchScore
	Distance int // Levenshtein distance for fuzzy matches
}

// FuzzyMatcher provides fuzzy string matching capabilities.
type FuzzyMatcher struct {
	// MaxDistance is the maximum Levenshtein distance for a fuzzy match.
	// Default is 2 for short strings, 3 for longer ones.
	MaxDistance int
}

// NewFuzzyMatcher creates a FuzzyMatcher with default settings.
func NewFuzzyMatcher() *FuzzyMatcher {
	return &FuzzyMatcher{
		MaxDistance: 3,
	}
}

// Match compares a target against a candidate and returns a score.
func (m *FuzzyMatcher) Match(target, candidate string) (MatchScore, int) {
	// Normalize both for comparison.
	targetNorm := normalizeForMatch(target)
	candidateNorm := normalizeForMatch(candidate)

	// Exact match.
	if targetNorm == candidateNorm {
		if target == candidate {
			return MatchExact, 0
		}
		return MatchCaseInsensitive, 0
	}

	// Prefix match (candidate starts with target).
	if strings.HasPrefix(candidateNorm, targetNorm) {
		return MatchPrefix, len(candidateNorm) - len(targetNorm)
	}

	// Fuzzy match using Levenshtein distance.
	maxDist := m.maxDistance(targetNorm)
	dist := levenshteinDistance(targetNorm, candidateNorm)
	if dist <= maxDist {
		return MatchFuzzy, dist
	}

	return MatchNone, dist
}

// maxDistance returns the maximum allowed Levenshtein distance based on string length.
func (m *FuzzyMatcher) maxDistance(s string) int {
	if m.MaxDistance > 0 {
		return m.MaxDistance
	}
	// Default: shorter strings get stricter matching.
	switch {
	case len(s) <= 4:
		return 1
	case len(s) <= 8:
		return 2
	default:
		return 3
	}
}

// FindBestMatches finds the best matching entries from a list of candidates.
// Returns matches sorted by score (best first), limited to maxResults.
func (m *FuzzyMatcher) FindBestMatches(target string, candidates []MatchResult, maxResults int) []MatchResult {
	targetName := normalizeForMatch(extractName(target))
	var matches []MatchResult

	for _, c := range candidates {
		candidateName := normalizeForMatch(extractName(c.Path))
		score, dist := m.Match(targetName, candidateName)

		if score > MatchNone {
			result := c
			result.Score = score
			result.Distance = dist
			result.Name = extractName(c.Path)
			matches = append(matches, result)
		}
	}

	// Sort by score (descending), then by distance (ascending).
	sortMatches(matches)

	if maxResults > 0 && len(matches) > maxResults {
		matches = matches[:maxResults]
	}

	return matches
}

// sortMatches sorts matches by score (best first), then by distance.
func sortMatches(matches []MatchResult) {
	// Simple bubble sort for small lists.
	for i := 0; i < len(matches); i++ {
		for j := i + 1; j < len(matches); j++ {
			if compareMatches(matches[j], matches[i]) < 0 {
				matches[i], matches[j] = matches[j], matches[i]
			}
		}
	}
}

// compareMatches returns negative if a is better than b.
func compareMatches(a, b MatchResult) int {
	// Higher score is better.
	if a.Score != b.Score {
		return int(b.Score) - int(a.Score)
	}
	// Lower distance is better.
	return a.Distance - b.Distance
}

// normalizeForMatch normalizes a string for fuzzy comparison.
func normalizeForMatch(s string) string {
	// Lowercase.
	s = strings.ToLower(s)

	// Remove common separators and normalize spaces.
	var result strings.Builder
	prevSpace := false
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			result.WriteRune(r)
			prevSpace = false
		} else if !prevSpace {
			result.WriteRune(' ')
			prevSpace = true
		}
	}
	return strings.TrimSpace(result.String())
}

// extractName extracts the file name without extension from a path.
func extractName(path string) string {
	name := filepath.Base(path)
	return strings.TrimSuffix(name, ".md")
}

// levenshteinDistance calculates the Levenshtein distance between two strings.
// This is the minimum number of single-character edits (insertions, deletions,
// or substitutions) required to change one string into the other.
func levenshteinDistance(s1, s2 string) int {
	if s1 == s2 {
		return 0
	}
	if len(s1) == 0 {
		return len(s2)
	}
	if len(s2) == 0 {
		return len(s1)
	}

	// Convert to runes for proper Unicode handling.
	r1 := []rune(s1)
	r2 := []rune(s2)

	// Create two work rows.
	prev := make([]int, len(r2)+1)
	curr := make([]int, len(r2)+1)

	// Initialize first row.
	for j := range prev {
		prev[j] = j
	}

	// Fill in the matrix.
	for i := 1; i <= len(r1); i++ {
		curr[0] = i

		for j := 1; j <= len(r2); j++ {
			cost := 0
			if r1[i-1] != r2[j-1] {
				cost = 1
			}

			curr[j] = min(
				prev[j]+1,      // Deletion
				curr[j-1]+1,    // Insertion
				prev[j-1]+cost, // Substitution
			)
		}

		// Swap rows.
		prev, curr = curr, prev
	}

	return prev[len(r2)]
}
