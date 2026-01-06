package state

import (
	"strings"
	"testing"
)

func TestHashContent_Basic(t *testing.T) {
	content := []byte(`---
title: Test Note
tags:
  - tag1
  - tag2
---
# Test Note

This is the body content.
`)

	hashes := HashContent(content)

	// All hashes should be non-empty
	if hashes.ContentHash == "" {
		t.Error("expected non-empty ContentHash")
	}
	if hashes.FrontmatterHash == "" {
		t.Error("expected non-empty FrontmatterHash")
	}
	if hashes.FullHash == "" {
		t.Error("expected non-empty FullHash")
	}

	// Hashes should be SHA256 hex strings (64 chars)
	if len(hashes.ContentHash) != 64 {
		t.Errorf("expected 64-char hash, got %d chars", len(hashes.ContentHash))
	}
}

func TestHashContent_NoFrontmatter(t *testing.T) {
	content := []byte(`# Just a Note

No frontmatter here.
`)

	hashes := HashContent(content)

	// Content hash should exist
	if hashes.ContentHash == "" {
		t.Error("expected non-empty ContentHash")
	}

	// Frontmatter hash should be empty
	if hashes.FrontmatterHash != "" {
		t.Errorf("expected empty FrontmatterHash, got %q", hashes.FrontmatterHash)
	}

	// Full hash should equal content hash when no frontmatter
	if hashes.FullHash != hashes.ContentHash {
		t.Error("FullHash should equal ContentHash when no frontmatter")
	}
}

func TestHashContent_OnlyFrontmatter(t *testing.T) {
	content := []byte(`---
title: Metadata Only
---
`)

	hashes := HashContent(content)

	// Frontmatter hash should exist
	if hashes.FrontmatterHash == "" {
		t.Error("expected non-empty FrontmatterHash")
	}

	// Content hash should be empty (no body)
	if hashes.ContentHash != "" {
		t.Errorf("expected empty ContentHash, got %q", hashes.ContentHash)
	}
}

func TestHashContent_EmptyContent(t *testing.T) {
	hashes := HashContent([]byte{})

	if hashes.ContentHash != "" {
		t.Error("expected empty ContentHash for empty input")
	}
	if hashes.FrontmatterHash != "" {
		t.Error("expected empty FrontmatterHash for empty input")
	}
	if hashes.FullHash != "" {
		t.Error("expected empty FullHash for empty input")
	}
}

func TestHashContent_WhitespaceNormalization(t *testing.T) {
	// Two versions with different whitespace should produce same hash
	content1 := []byte("# Title\n\nParagraph one.\n\nParagraph two.")
	content2 := []byte("# Title  \n\nParagraph one.   \n\nParagraph two.  ")

	hashes1 := HashContent(content1)
	hashes2 := HashContent(content2)

	if hashes1.ContentHash != hashes2.ContentHash {
		t.Error("trailing whitespace should not affect hash")
	}
}

func TestHashContent_LineEndingNormalization(t *testing.T) {
	// Unix and Windows line endings should produce same hash
	contentUnix := []byte("Line 1\nLine 2\nLine 3")
	contentWindows := []byte("Line 1\r\nLine 2\r\nLine 3")
	contentOldMac := []byte("Line 1\rLine 2\rLine 3")

	hashUnix := HashContent(contentUnix)
	hashWindows := HashContent(contentWindows)
	hashOldMac := HashContent(contentOldMac)

	if hashUnix.ContentHash != hashWindows.ContentHash {
		t.Error("Unix and Windows line endings should produce same hash")
	}
	if hashUnix.ContentHash != hashOldMac.ContentHash {
		t.Error("Unix and old Mac line endings should produce same hash")
	}
}

func TestHashContent_MultipleBlankLinesNormalization(t *testing.T) {
	// Multiple blank lines should collapse to 2 newlines max
	content1 := []byte("Para 1\n\nPara 2")
	content2 := []byte("Para 1\n\n\n\n\nPara 2")

	hashes1 := HashContent(content1)
	hashes2 := HashContent(content2)

	if hashes1.ContentHash != hashes2.ContentHash {
		t.Error("multiple blank lines should normalize to same hash")
	}
}

func TestHashContent_FrontmatterChangeOnly(t *testing.T) {
	content1 := []byte(`---
title: Original Title
---
# Body content
`)
	content2 := []byte(`---
title: Changed Title
---
# Body content
`)

	hashes1 := HashContent(content1)
	hashes2 := HashContent(content2)

	// Body should be same
	if hashes1.ContentHash != hashes2.ContentHash {
		t.Error("body content unchanged, ContentHash should match")
	}

	// Frontmatter should differ
	if hashes1.FrontmatterHash == hashes2.FrontmatterHash {
		t.Error("frontmatter changed, FrontmatterHash should differ")
	}

	// Full hash should differ
	if hashes1.FullHash == hashes2.FullHash {
		t.Error("content changed, FullHash should differ")
	}
}

func TestHashContent_BodyChangeOnly(t *testing.T) {
	content1 := []byte(`---
title: Same Title
---
# Original body
`)
	content2 := []byte(`---
title: Same Title
---
# Changed body
`)

	hashes1 := HashContent(content1)
	hashes2 := HashContent(content2)

	// Frontmatter should be same
	if hashes1.FrontmatterHash != hashes2.FrontmatterHash {
		t.Error("frontmatter unchanged, FrontmatterHash should match")
	}

	// Body should differ
	if hashes1.ContentHash == hashes2.ContentHash {
		t.Error("body changed, ContentHash should differ")
	}
}

func TestHashContentRaw(t *testing.T) {
	content := []byte("test content")
	hash := HashContentRaw(content)

	if len(hash) != 64 {
		t.Errorf("expected 64-char SHA256 hex, got %d chars", len(hash))
	}

	// Same content should produce same hash
	hash2 := HashContentRaw(content)
	if hash != hash2 {
		t.Error("same content should produce same raw hash")
	}

	// Different content should produce different hash
	hash3 := HashContentRaw([]byte("different content"))
	if hash == hash3 {
		t.Error("different content should produce different raw hash")
	}
}

func TestSplitFrontmatter_Valid(t *testing.T) {
	content := []byte(`---
title: Test
---
body content`)

	fm, body := splitFrontmatter(content)

	if string(fm) != "title: Test" {
		t.Errorf("expected frontmatter 'title: Test', got %q", string(fm))
	}
	if string(body) != "body content" {
		t.Errorf("expected body 'body content', got %q", string(body))
	}
}

func TestSplitFrontmatter_NoFrontmatter(t *testing.T) {
	content := []byte("Just plain markdown")

	fm, body := splitFrontmatter(content)

	if fm != nil {
		t.Errorf("expected nil frontmatter, got %q", string(fm))
	}
	if string(body) != "Just plain markdown" {
		t.Errorf("expected body to equal content")
	}
}

func TestSplitFrontmatter_NoClosingDelimiter(t *testing.T) {
	content := []byte(`---
title: Test
no closing delimiter`)

	fm, body := splitFrontmatter(content)

	// Should return content as body when frontmatter isn't closed
	if fm != nil {
		t.Errorf("expected nil frontmatter for unclosed, got %q", string(fm))
	}
	if string(body) != string(content) {
		t.Error("body should equal original content when frontmatter unclosed")
	}
}

func TestSplitFrontmatter_FrontmatterAtEndOfFile(t *testing.T) {
	content := []byte(`---
title: Test
---`)

	fm, body := splitFrontmatter(content)

	if string(fm) != "title: Test" {
		t.Errorf("expected frontmatter 'title: Test', got %q", string(fm))
	}
	if body != nil {
		t.Errorf("expected nil body, got %q", string(body))
	}
}

func TestSplitFrontmatter_EmptyBody(t *testing.T) {
	content := []byte(`---
title: Test
---
`)

	fm, body := splitFrontmatter(content)

	if string(fm) != "title: Test" {
		t.Errorf("expected frontmatter 'title: Test', got %q", string(fm))
	}
	// Body after trailing newline should be empty string, not nil
	if len(body) != 0 {
		t.Errorf("expected empty body, got %q", string(body))
	}
}

func TestSplitFrontmatter_NotAtStart(t *testing.T) {
	content := []byte(`Some text first
---
title: Test
---`)

	fm, _ := splitFrontmatter(content)

	// Frontmatter must be at start of file
	if fm != nil {
		t.Error("frontmatter not at start should return nil")
	}
}

func TestSplitFrontmatter_NoNewlineAfterOpeningDelimiter(t *testing.T) {
	content := []byte(`---title: inline
---
body`)

	fm, body := splitFrontmatter(content)

	// Opening --- must be followed by newline
	if fm != nil {
		t.Error("frontmatter without newline after opening should return nil")
	}
	if string(body) != string(content) {
		t.Error("body should equal original content")
	}
}

func TestNormalizeContent(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "trailing whitespace removed",
			input:    "line 1  \nline 2\t\nline 3",
			expected: "line 1\nline 2\nline 3",
		},
		{
			name:     "windows line endings",
			input:    "line 1\r\nline 2\r\nline 3",
			expected: "line 1\nline 2\nline 3",
		},
		{
			name:     "old mac line endings",
			input:    "line 1\rline 2\rline 3",
			expected: "line 1\nline 2\nline 3",
		},
		{
			name:     "multiple blank lines collapsed",
			input:    "para 1\n\n\n\npara 2",
			expected: "para 1\n\npara 2",
		},
		{
			name:     "leading whitespace trimmed",
			input:    "\n\n  content",
			expected: "content",
		},
		{
			name:     "trailing whitespace trimmed",
			input:    "content  \n\n",
			expected: "content",
		},
		{
			name:     "empty input",
			input:    "",
			expected: "",
		},
		{
			name:     "whitespace only",
			input:    "   \n\n\t\t\n   ",
			expected: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := normalizeContent([]byte(tc.input))
			if tc.expected == "" {
				if result != nil {
					t.Errorf("expected nil, got %q", string(result))
				}
			} else {
				if string(result) != tc.expected {
					t.Errorf("expected %q, got %q", tc.expected, string(result))
				}
			}
		})
	}
}

func TestHasContentChanged(t *testing.T) {
	old := ContentHashes{
		ContentHash:     "abc123",
		FrontmatterHash: "def456",
	}

	// Same hashes - no change
	same := ContentHashes{
		ContentHash:     "abc123",
		FrontmatterHash: "def456",
	}
	if HasContentChanged(old, same) {
		t.Error("identical hashes should not report change")
	}

	// Different content hash
	diffContent := ContentHashes{
		ContentHash:     "different",
		FrontmatterHash: "def456",
	}
	if !HasContentChanged(old, diffContent) {
		t.Error("different content hash should report change")
	}

	// Different frontmatter hash
	diffFM := ContentHashes{
		ContentHash:     "abc123",
		FrontmatterHash: "different",
	}
	if !HasContentChanged(old, diffFM) {
		t.Error("different frontmatter hash should report change")
	}
}

func TestHasBodyChanged(t *testing.T) {
	old := ContentHashes{ContentHash: "abc123"}

	if HasBodyChanged(old, ContentHashes{ContentHash: "abc123"}) {
		t.Error("same content hash should not report body change")
	}

	if !HasBodyChanged(old, ContentHashes{ContentHash: "different"}) {
		t.Error("different content hash should report body change")
	}
}

func TestHasFrontmatterChanged(t *testing.T) {
	old := ContentHashes{FrontmatterHash: "def456"}

	if HasFrontmatterChanged(old, ContentHashes{FrontmatterHash: "def456"}) {
		t.Error("same frontmatter hash should not report change")
	}

	if !HasFrontmatterChanged(old, ContentHashes{FrontmatterHash: "different"}) {
		t.Error("different frontmatter hash should report change")
	}
}

func TestHashesFromState(t *testing.T) {
	state := &SyncState{
		ContentHash:     "content-hash-value",
		FrontmatterHash: "frontmatter-hash-value",
	}

	hashes := HashesFromState(state)

	if hashes.ContentHash != "content-hash-value" {
		t.Errorf("expected ContentHash 'content-hash-value', got %q", hashes.ContentHash)
	}
	if hashes.FrontmatterHash != "frontmatter-hash-value" {
		t.Errorf("expected FrontmatterHash 'frontmatter-hash-value', got %q", hashes.FrontmatterHash)
	}
	if hashes.FullHash != "" {
		t.Error("FullHash should be empty (not stored in state)")
	}
}

func TestComputeHash_Deterministic(t *testing.T) {
	content := []byte("test content for hashing")

	// Multiple calls should produce same result
	hash1 := computeHash(content)
	hash2 := computeHash(content)
	hash3 := computeHash(content)

	if hash1 != hash2 || hash2 != hash3 {
		t.Error("computeHash should be deterministic")
	}
}

func TestComputeHash_KnownValue(t *testing.T) {
	// Verify against known SHA256 hash
	content := []byte("hello world")
	hash := computeHash(content)

	// SHA256 of "hello world" is:
	// b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9
	expected := "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9"
	if hash != expected {
		t.Errorf("expected known hash %q, got %q", expected, hash)
	}
}

func TestHashContent_LargeFile(t *testing.T) {
	// Create a large file with lots of content
	var sb strings.Builder
	sb.WriteString("---\ntitle: Large File\n---\n")
	for i := 0; i < 10000; i++ {
		sb.WriteString("This is line number ")
		sb.WriteString(string(rune('0' + i%10)))
		sb.WriteString(" of the large file content.\n")
	}

	content := []byte(sb.String())
	hashes := HashContent(content)

	// Should complete without error and produce valid hashes
	if hashes.ContentHash == "" {
		t.Error("expected non-empty ContentHash for large file")
	}
	if hashes.FrontmatterHash == "" {
		t.Error("expected non-empty FrontmatterHash for large file")
	}
	if hashes.FullHash == "" {
		t.Error("expected non-empty FullHash for large file")
	}
}
