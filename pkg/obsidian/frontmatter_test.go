package obsidian

import (
	"testing"
	"time"
)

func TestParseFrontmatter_Valid(t *testing.T) {
	content := []byte(`---
title: My Note
tags:
  - tag1
  - tag2
date: 2024-01-15
---
# My Note

This is the body content.
`)

	fm, body, err := ParseFrontmatter(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Check title
	if title := fm.GetString("title"); title != "My Note" {
		t.Errorf("expected title 'My Note', got %q", title)
	}

	// Check tags
	tags := fm.GetStringSlice("tags")
	if len(tags) != 2 || tags[0] != "tag1" || tags[1] != "tag2" {
		t.Errorf("unexpected tags: %v", tags)
	}

	// Check body starts correctly
	expectedBody := "# My Note\n\nThis is the body content.\n"
	if string(body) != expectedBody {
		t.Errorf("expected body %q, got %q", expectedBody, string(body))
	}
}

func TestParseFrontmatter_NoFrontmatter(t *testing.T) {
	content := []byte("# Just a regular markdown file\n\nNo frontmatter here.")

	fm, body, err := ParseFrontmatter(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(fm) != 0 {
		t.Errorf("expected empty frontmatter, got %v", fm)
	}

	if string(body) != string(content) {
		t.Errorf("body should equal original content")
	}
}

func TestParseFrontmatter_EmptyFrontmatter(t *testing.T) {
	// Note: With just "---\n---", there's no content between delimiters,
	// so the parser returns the whole content as body (no match for \n---\n)
	content := []byte(`---

---
# Body content
`)

	fm, body, err := ParseFrontmatter(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(fm) != 0 {
		t.Errorf("expected empty frontmatter, got %v", fm)
	}

	if string(body) != "# Body content\n" {
		t.Errorf("unexpected body: %q", string(body))
	}
}

func TestParseFrontmatter_UnclosedDelimiter(t *testing.T) {
	content := []byte(`---
title: Unclosed
This has no closing delimiter.
`)

	fm, body, err := ParseFrontmatter(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should return original content when delimiter isn't closed properly
	if len(fm) != 0 {
		t.Errorf("expected empty frontmatter for unclosed delimiter")
	}
	if string(body) != string(content) {
		t.Error("body should equal original content for unclosed delimiter")
	}
}

func TestParseFrontmatter_AtEndOfFile(t *testing.T) {
	// Frontmatter that ends with newline after closing delimiter works fine
	content := []byte(`---
title: Test
---
`)

	fm, body, err := ParseFrontmatter(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if fm.GetString("title") != "Test" {
		t.Errorf("expected title 'Test', got %q", fm.GetString("title"))
	}

	// Body should be empty after frontmatter
	if len(body) != 0 {
		t.Errorf("expected empty body, got %q", string(body))
	}
}

func TestParseFrontmatter_InvalidYAML(t *testing.T) {
	content := []byte(`---
title: [invalid yaml
  missing bracket
---
body
`)

	_, _, err := ParseFrontmatter(content)
	if err == nil {
		t.Error("expected error for invalid YAML")
	}
}

func TestSerializeFrontmatter(t *testing.T) {
	fm := Frontmatter{
		"title": "Test Note",
		"tags":  []string{"tag1", "tag2"},
	}

	data, err := SerializeFrontmatter(fm)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should start and end with ---
	if len(data) < 7 {
		t.Fatalf("output too short: %q", string(data))
	}

	// Parse it back
	parsed, _, err := ParseFrontmatter(append(data, []byte("body\n")...))
	if err != nil {
		t.Fatalf("failed to parse serialized frontmatter: %v", err)
	}

	if parsed.GetString("title") != "Test Note" {
		t.Errorf("round-trip failed for title")
	}
}

func TestSerializeFrontmatter_Empty(t *testing.T) {
	fm := Frontmatter{}

	data, err := SerializeFrontmatter(fm)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if data != nil {
		t.Errorf("expected nil for empty frontmatter, got %q", string(data))
	}
}

func TestFrontmatter_GetString(t *testing.T) {
	fm := Frontmatter{
		"title":    "My Title",
		"notastr":  123,
		"nilvalue": nil,
	}

	tests := []struct {
		key      string
		expected string
	}{
		{"title", "My Title"},
		{"notastr", ""},
		{"nilvalue", ""},
		{"missing", ""},
	}

	for _, tc := range tests {
		t.Run(tc.key, func(t *testing.T) {
			result := fm.GetString(tc.key)
			if result != tc.expected {
				t.Errorf("expected %q, got %q", tc.expected, result)
			}
		})
	}
}

func TestFrontmatter_GetStringSlice(t *testing.T) {
	fm := Frontmatter{
		"tags":      []any{"tag1", "tag2", "tag3"},
		"single":    "just one",
		"strslice":  []string{"a", "b"},
		"mixed":     []any{"str", 123, "another"},
		"notslice":  42,
		"nilvalue":  nil,
	}

	tests := []struct {
		name     string
		key      string
		expected []string
	}{
		{"any slice", "tags", []string{"tag1", "tag2", "tag3"}},
		{"single string", "single", []string{"just one"}},
		{"string slice", "strslice", []string{"a", "b"}},
		{"mixed types", "mixed", []string{"str", "another"}}, // non-strings filtered
		{"not slice", "notslice", nil},
		{"nil value", "nilvalue", nil},
		{"missing", "missing", nil},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := fm.GetStringSlice(tc.key)
			if len(result) != len(tc.expected) {
				t.Errorf("expected %d items, got %d: %v", len(tc.expected), len(result), result)
				return
			}
			for i, v := range tc.expected {
				if result[i] != v {
					t.Errorf("item %d: expected %q, got %q", i, v, result[i])
				}
			}
		})
	}
}

func TestFrontmatter_GetInt(t *testing.T) {
	fm := Frontmatter{
		"count":    42,
		"floatval": 3.14,
		"strval":   "123",
		"nilvalue": nil,
	}

	tests := []struct {
		key      string
		expected int
	}{
		{"count", 42},
		{"floatval", 3},
		{"strval", 0},
		{"nilvalue", 0},
		{"missing", 0},
	}

	for _, tc := range tests {
		t.Run(tc.key, func(t *testing.T) {
			result := fm.GetInt(tc.key)
			if result != tc.expected {
				t.Errorf("expected %d, got %d", tc.expected, result)
			}
		})
	}
}

func TestFrontmatter_GetBool(t *testing.T) {
	fm := Frontmatter{
		"published": true,
		"draft":     false,
		"strval":    "true",
		"nilvalue":  nil,
	}

	tests := []struct {
		key      string
		expected bool
	}{
		{"published", true},
		{"draft", false},
		{"strval", false}, // string "true" is not bool
		{"nilvalue", false},
		{"missing", false},
	}

	for _, tc := range tests {
		t.Run(tc.key, func(t *testing.T) {
			result := fm.GetBool(tc.key)
			if result != tc.expected {
				t.Errorf("expected %v, got %v", tc.expected, result)
			}
		})
	}
}

func TestFrontmatter_GetTime(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	fm := Frontmatter{
		"date":      "2024-01-15",
		"datetime":  "2024-01-15T10:30:00",
		"rfc3339":   "2024-01-15T10:30:00Z",
		"slash":     "2024/06/20",
		"usformat":  "12/25/2024",
		"timetype":  now,
		"invalid":   "not a date",
		"intval":    12345,
	}

	tests := []struct {
		name      string
		key       string
		expectZero bool
		expectYear int
	}{
		{"date format", "date", false, 2024},
		{"datetime format", "datetime", false, 2024},
		{"rfc3339 format", "rfc3339", false, 2024},
		{"slash format", "slash", false, 2024},
		{"US format", "usformat", false, 2024},
		{"time.Time type", "timetype", false, now.Year()},
		{"invalid string", "invalid", true, 0},
		{"int value", "intval", true, 0},
		{"missing key", "missing", true, 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := fm.GetTime(tc.key)
			if tc.expectZero {
				if !result.IsZero() {
					t.Errorf("expected zero time, got %v", result)
				}
			} else {
				if result.IsZero() {
					t.Error("expected non-zero time")
				} else if result.Year() != tc.expectYear {
					t.Errorf("expected year %d, got %d", tc.expectYear, result.Year())
				}
			}
		})
	}
}

func TestFrontmatter_Set(t *testing.T) {
	fm := make(Frontmatter)

	fm.Set("title", "New Title")
	if fm["title"] != "New Title" {
		t.Errorf("Set failed: expected 'New Title', got %v", fm["title"])
	}

	// Overwrite
	fm.Set("title", "Updated Title")
	if fm["title"] != "Updated Title" {
		t.Errorf("Set overwrite failed")
	}
}

func TestFrontmatter_Delete(t *testing.T) {
	fm := Frontmatter{
		"keep":   "value",
		"delete": "value",
	}

	fm.Delete("delete")

	if _, ok := fm["delete"]; ok {
		t.Error("Delete failed: key still exists")
	}
	if fm["keep"] != "value" {
		t.Error("Delete removed wrong key")
	}

	// Delete non-existent key should not panic
	fm.Delete("nonexistent")
}

func TestFrontmatter_Has(t *testing.T) {
	fm := Frontmatter{
		"exists":   "value",
		"nilvalue": nil,
	}

	if !fm.Has("exists") {
		t.Error("Has should return true for existing key")
	}
	if !fm.Has("nilvalue") {
		t.Error("Has should return true for nil value key")
	}
	if fm.Has("missing") {
		t.Error("Has should return false for missing key")
	}
}

func TestFrontmatter_Merge(t *testing.T) {
	fm := Frontmatter{
		"a": 1,
		"b": 2,
	}
	other := Frontmatter{
		"b": 3, // Override
		"c": 4, // New
	}

	fm.Merge(other)

	if fm["a"] != 1 {
		t.Error("Merge should preserve non-conflicting keys")
	}
	if fm["b"] != 3 {
		t.Error("Merge should override with other's values")
	}
	if fm["c"] != 4 {
		t.Error("Merge should add new keys from other")
	}
}

func TestFrontmatter_Clone(t *testing.T) {
	fm := Frontmatter{
		"title": "Original",
		"tags":  []string{"tag1", "tag2"},
	}

	clone := fm.Clone()

	// Modify original
	fm["title"] = "Modified"

	// Clone should be unchanged
	if clone.GetString("title") != "Original" {
		t.Error("Clone should be independent of original")
	}
}

func TestFrontmatter_Tags(t *testing.T) {
	fm := Frontmatter{
		"tags": []any{"work", "important"},
	}

	tags := fm.Tags()
	if len(tags) != 2 || tags[0] != "work" || tags[1] != "important" {
		t.Errorf("unexpected tags: %v", tags)
	}

	// Empty frontmatter
	empty := Frontmatter{}
	if len(empty.Tags()) != 0 {
		t.Error("Tags should return nil for missing key")
	}
}

func TestFrontmatter_Aliases(t *testing.T) {
	fm := Frontmatter{
		"aliases": []any{"alias1", "alias2"},
	}

	aliases := fm.Aliases()
	if len(aliases) != 2 || aliases[0] != "alias1" {
		t.Errorf("unexpected aliases: %v", aliases)
	}
}

func TestFrontmatter_Title(t *testing.T) {
	fm := Frontmatter{
		"title": "My Document",
	}

	if fm.Title() != "My Document" {
		t.Errorf("unexpected title: %s", fm.Title())
	}

	// Missing title
	empty := Frontmatter{}
	if empty.Title() != "" {
		t.Error("Title should return empty string for missing key")
	}
}

func TestParseFrontmatter_ComplexYAML(t *testing.T) {
	content := []byte(`---
title: Complex Note
author:
  name: John Doe
  email: john@example.com
metadata:
  created: 2024-01-15
  updated: 2024-01-20
  version: 1.2
tags:
  - nested/tag
  - multi-word tag
published: true
count: 42
---
# Content here
`)

	fm, body, err := ParseFrontmatter(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Check string
	if fm.GetString("title") != "Complex Note" {
		t.Error("failed to get title")
	}

	// Check bool
	if !fm.GetBool("published") {
		t.Error("failed to get published bool")
	}

	// Check int
	if fm.GetInt("count") != 42 {
		t.Error("failed to get count int")
	}

	// Check nested map exists
	if !fm.Has("author") {
		t.Error("missing author map")
	}

	// Check body
	if string(body) != "# Content here\n" {
		t.Errorf("unexpected body: %q", string(body))
	}
}
