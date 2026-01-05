package parser

import (
	"testing"
)

func TestNew(t *testing.T) {
	p := New()
	if p == nil {
		t.Fatal("New() returned nil")
	}
	if p.md == nil {
		t.Fatal("Parser.md is nil")
	}
}

func TestParse_BasicMarkdown(t *testing.T) {
	p := New()

	content := []byte(`# Heading 1

This is a paragraph with **bold** and *italic* text.

## Heading 2

- Item 1
- Item 2
- Item 3

### Heading 3

1. Numbered item
2. Another item
`)

	note, err := p.Parse("test.md", content)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	if note.Path != "test.md" {
		t.Errorf("Path = %q, want %q", note.Path, "test.md")
	}

	if note.AST == nil {
		t.Error("AST is nil")
	}

	if len(note.Source) == 0 {
		t.Error("Source is empty")
	}
}

func TestParse_WithFrontmatter(t *testing.T) {
	p := New()

	content := []byte(`---
title: Test Note
tags:
  - tag1
  - tag2
status: draft
---

# Content

This is the body.
`)

	note, err := p.Parse("test.md", content)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	// Check frontmatter parsing.
	if note.Frontmatter["title"] != "Test Note" {
		t.Errorf("Frontmatter title = %v, want %q", note.Frontmatter["title"], "Test Note")
	}

	if note.Frontmatter["status"] != "draft" {
		t.Errorf("Frontmatter status = %v, want %q", note.Frontmatter["status"], "draft")
	}

	// Check tags from frontmatter.
	if len(note.Tags) != 2 {
		t.Errorf("Tags count = %d, want 2", len(note.Tags))
	}
}

func TestParse_WikiLinks(t *testing.T) {
	p := New()

	content := []byte(`# Links

See [[Other Note]] for more info.

Also check [[Target|Custom Alias]].

Reference [[Note#Heading]] and [[Note^block-id]].
`)

	note, err := p.Parse("test.md", content)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	if len(note.WikiLinks) < 3 {
		t.Fatalf("WikiLinks count = %d, want at least 3", len(note.WikiLinks))
	}

	// Check first link (simple).
	found := false
	for _, link := range note.WikiLinks {
		if link.Target == "Other Note" {
			found = true
			break
		}
	}
	if !found {
		t.Error("WikiLink 'Other Note' not found")
	}

	// Check link with heading.
	found = false
	for _, link := range note.WikiLinks {
		if link.Target == "Note" && link.Heading == "Heading" {
			found = true
			break
		}
	}
	if !found {
		t.Error("WikiLink with heading 'Note#Heading' not found")
	}

	// Check link with block reference.
	found = false
	for _, link := range note.WikiLinks {
		if link.Target == "Note" && link.Block == "block-id" {
			found = true
			break
		}
	}
	if !found {
		t.Error("WikiLink with block reference 'Note^block-id' not found")
	}
}

func TestParse_Tags(t *testing.T) {
	p := New()

	content := []byte(`---
tags:
  - frontmatter-tag
---

# Tags Test

This has #inline-tag and #another-tag.
`)

	note, err := p.Parse("test.md", content)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	// Should have both frontmatter and inline tags.
	if len(note.Tags) < 2 {
		t.Errorf("Tags count = %d, want at least 2", len(note.Tags))
	}

	// Check for frontmatter tag.
	found := false
	for _, tag := range note.Tags {
		if tag == "frontmatter-tag" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Frontmatter tag 'frontmatter-tag' not found")
	}
}

func TestParse_Embeds(t *testing.T) {
	p := New()

	content := []byte(`# Embeds Test

![[image.png]]

![[document.pdf]]

![[Other Note]]

![[image.jpg|100]]

![[banner.png|800x200]]
`)

	note, err := p.Parse("test.md", content)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	if len(note.Embeds) < 4 {
		t.Fatalf("Embeds count = %d, want at least 4", len(note.Embeds))
	}

	// Check image embed.
	var imageEmbed *Embed
	for i, e := range note.Embeds {
		if e.Target == "image.png" {
			imageEmbed = &note.Embeds[i]
			break
		}
	}
	if imageEmbed == nil {
		t.Fatal("Image embed 'image.png' not found")
	}
	if !imageEmbed.IsImage {
		t.Error("image.png should be marked as image")
	}

	// Check PDF embed.
	var pdfEmbed *Embed
	for i, e := range note.Embeds {
		if e.Target == "document.pdf" {
			pdfEmbed = &note.Embeds[i]
			break
		}
	}
	if pdfEmbed == nil {
		t.Fatal("PDF embed 'document.pdf' not found")
	}
	if !pdfEmbed.IsPDF {
		t.Error("document.pdf should be marked as PDF")
	}

	// Check embed with dimensions.
	var dimensionEmbed *Embed
	for i, e := range note.Embeds {
		if e.Target == "banner.png" {
			dimensionEmbed = &note.Embeds[i]
			break
		}
	}
	if dimensionEmbed == nil {
		t.Fatal("Embed 'banner.png' with dimensions not found")
	}
	if dimensionEmbed.Width != 800 || dimensionEmbed.Height != 200 {
		t.Errorf("Embed dimensions = %dx%d, want 800x200", dimensionEmbed.Width, dimensionEmbed.Height)
	}
}

func TestParse_DataviewQueries(t *testing.T) {
	p := New()

	content := []byte("# Dataview Test\n\n```dataview\nTABLE file.name\nFROM #tag\n```\n\nInline: `=this.file.name`\n\n```dataviewjs\ndv.list([1,2,3])\n```\n")

	note, err := p.Parse("test.md", content)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	if len(note.DataviewQueries) != 3 {
		t.Fatalf("DataviewQueries count = %d, want 3", len(note.DataviewQueries))
	}

	// Check TABLE query.
	found := false
	for _, q := range note.DataviewQueries {
		if q.Type == QueryTypeTable {
			found = true
			break
		}
	}
	if !found {
		t.Error("TABLE query not found")
	}

	// Check inline query.
	found = false
	for _, q := range note.DataviewQueries {
		if q.Type == QueryTypeInline && q.IsInline {
			found = true
			break
		}
	}
	if !found {
		t.Error("Inline query not found")
	}

	// Check JS query.
	found = false
	for _, q := range note.DataviewQueries {
		if q.Type == QueryTypeJS {
			found = true
			break
		}
	}
	if !found {
		t.Error("JS query not found")
	}
}

func TestParse_EmptyContent(t *testing.T) {
	p := New()

	note, err := p.Parse("empty.md", []byte(""))
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	if note.Path != "empty.md" {
		t.Errorf("Path = %q, want %q", note.Path, "empty.md")
	}

	if len(note.Frontmatter) != 0 {
		t.Errorf("Frontmatter should be empty, got %v", note.Frontmatter)
	}
}

func TestParse_FrontmatterOnly(t *testing.T) {
	p := New()

	content := []byte(`---
title: Only Frontmatter
---
`)

	note, err := p.Parse("test.md", content)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	if note.Frontmatter["title"] != "Only Frontmatter" {
		t.Errorf("Frontmatter title = %v, want %q", note.Frontmatter["title"], "Only Frontmatter")
	}
}

// Frontmatter edge cases tests.

func TestExtractFrontmatter_Valid(t *testing.T) {
	content := []byte(`---
title: Test
date: 2024-01-15
---

Body content here.
`)

	fm, body, err := extractFrontmatter(content)
	if err != nil {
		t.Fatalf("extractFrontmatter() error: %v", err)
	}

	if fm["title"] != "Test" {
		t.Errorf("title = %v, want %q", fm["title"], "Test")
	}

	if len(body) == 0 {
		t.Error("body should not be empty")
	}
}

func TestExtractFrontmatter_NoFrontmatter(t *testing.T) {
	content := []byte(`# Just a heading

No frontmatter here.
`)

	fm, body, err := extractFrontmatter(content)
	if err != nil {
		t.Fatalf("extractFrontmatter() error: %v", err)
	}

	if len(fm) != 0 {
		t.Errorf("frontmatter should be empty, got %v", fm)
	}

	if string(body) != string(content) {
		t.Error("body should equal original content when no frontmatter")
	}
}

func TestExtractFrontmatter_Empty(t *testing.T) {
	content := []byte(`---
---

Empty frontmatter.
`)

	fm, body, err := extractFrontmatter(content)
	if err != nil {
		t.Fatalf("extractFrontmatter() error: %v", err)
	}

	if len(fm) != 0 {
		t.Errorf("frontmatter should be empty, got %v", fm)
	}

	if len(body) == 0 {
		t.Error("body should not be empty")
	}
}

func TestExtractFrontmatter_MalformedYAML_Strict(t *testing.T) {
	content := []byte(`---
title: Test
invalid yaml here [[[
key: value
---

Body.
`)

	_, _, err := extractFrontmatter(content)
	if err == nil {
		t.Error("extractFrontmatter() should return error for malformed YAML")
	}

	// Check it's a FrontmatterError.
	if _, ok := err.(*FrontmatterError); !ok {
		t.Errorf("error should be FrontmatterError, got %T", err)
	}
}

func TestExtractFrontmatter_MalformedYAML_Lenient(t *testing.T) {
	content := []byte(`---
title: Test
key: value
invalid: [[[
---

Body.
`)

	fm, body, err := extractFrontmatterWithMode(content, FrontmatterLenient)
	if err != nil {
		t.Fatalf("extractFrontmatterWithMode(Lenient) error: %v", err)
	}

	// In lenient mode, we should get partial results.
	if len(fm) == 0 && len(body) == 0 {
		t.Error("lenient mode should recover some content")
	}
}

func TestExtractFrontmatter_UnclosedBlock_Strict(t *testing.T) {
	content := []byte(`---
title: Test
key: value
`)

	_, _, err := extractFrontmatter(content)
	if err == nil {
		t.Error("extractFrontmatter() should return error for unclosed block")
	}

	fmErr, ok := err.(*FrontmatterError)
	if !ok {
		t.Errorf("error should be FrontmatterError, got %T", err)
	}

	if fmErr.Message == "" {
		t.Error("FrontmatterError.Message should not be empty")
	}
}

func TestExtractFrontmatter_UnclosedBlock_Lenient(t *testing.T) {
	content := []byte(`---
title: Test
key: value
`)

	fm, body, err := extractFrontmatterWithMode(content, FrontmatterLenient)
	if err != nil {
		t.Fatalf("extractFrontmatterWithMode(Lenient) error: %v", err)
	}

	// In lenient mode, content should be treated as body.
	if len(fm) != 0 {
		t.Errorf("frontmatter should be empty in lenient mode with unclosed block, got %v", fm)
	}

	if len(body) == 0 {
		t.Error("body should contain original content in lenient mode")
	}
}

func TestExtractFrontmatter_WindowsLineEndings(t *testing.T) {
	content := []byte("---\r\ntitle: Test\r\n---\r\n\r\nBody.\r\n")

	fm, body, err := extractFrontmatter(content)
	if err != nil {
		t.Fatalf("extractFrontmatter() error: %v", err)
	}

	if fm["title"] != "Test" {
		t.Errorf("title = %v, want %q", fm["title"], "Test")
	}

	if len(body) == 0 {
		t.Error("body should not be empty")
	}
}

func TestExtractFrontmatter_NestedYAML(t *testing.T) {
	content := []byte(`---
metadata:
  author: John
  date: 2024-01-15
  nested:
    deep: value
tags:
  - tag1
  - tag2
---

Body.
`)

	fm, _, err := extractFrontmatter(content)
	if err != nil {
		t.Fatalf("extractFrontmatter() error: %v", err)
	}

	metadata, ok := fm["metadata"].(map[string]any)
	if !ok {
		t.Fatalf("metadata should be a map, got %T", fm["metadata"])
	}

	if metadata["author"] != "John" {
		t.Errorf("metadata.author = %v, want %q", metadata["author"], "John")
	}
}

// Embed helper function tests.

func TestIsImageEmbed(t *testing.T) {
	tests := []struct {
		target string
		want   bool
	}{
		{"image.png", true},
		{"photo.jpg", true},
		{"photo.JPEG", true},
		{"banner.gif", true},
		{"icon.svg", true},
		{"photo.webp", true},
		{"document.pdf", false},
		{"song.mp3", false},
		{"note", false},
		{"Image.PNG", true},
	}

	for _, tt := range tests {
		t.Run(tt.target, func(t *testing.T) {
			got := isImageEmbed(tt.target)
			if got != tt.want {
				t.Errorf("isImageEmbed(%q) = %v, want %v", tt.target, got, tt.want)
			}
		})
	}
}

func TestIsPDFEmbed(t *testing.T) {
	tests := []struct {
		target string
		want   bool
	}{
		{"document.pdf", true},
		{"Document.PDF", true},
		{"image.png", false},
		{"note", false},
	}

	for _, tt := range tests {
		t.Run(tt.target, func(t *testing.T) {
			got := isPDFEmbed(tt.target)
			if got != tt.want {
				t.Errorf("isPDFEmbed(%q) = %v, want %v", tt.target, got, tt.want)
			}
		})
	}
}

func TestIsAudioEmbed(t *testing.T) {
	tests := []struct {
		target string
		want   bool
	}{
		{"song.mp3", true},
		{"audio.wav", true},
		{"music.ogg", true},
		{"podcast.m4a", true},
		{"image.png", false},
		{"note", false},
	}

	for _, tt := range tests {
		t.Run(tt.target, func(t *testing.T) {
			got := isAudioEmbed(tt.target)
			if got != tt.want {
				t.Errorf("isAudioEmbed(%q) = %v, want %v", tt.target, got, tt.want)
			}
		})
	}
}

func TestIsVideoEmbed(t *testing.T) {
	tests := []struct {
		target string
		want   bool
	}{
		{"movie.mp4", true},
		{"video.mkv", true},
		{"clip.avi", true},
		{"recording.mov", true},
		{"image.png", false},
		{"note", false},
	}

	for _, tt := range tests {
		t.Run(tt.target, func(t *testing.T) {
			got := isVideoEmbed(tt.target)
			if got != tt.want {
				t.Errorf("isVideoEmbed(%q) = %v, want %v", tt.target, got, tt.want)
			}
		})
	}
}

func TestParseEmbedDimensions(t *testing.T) {
	tests := []struct {
		input      string
		wantTarget string
		wantWidth  int
		wantHeight int
	}{
		{"image.png", "image.png", 0, 0},
		{"image.png|100", "image.png", 100, 0},
		{"image.png|100x200", "image.png", 100, 200},
		{"path/to/image.png|800x600", "path/to/image.png", 800, 600},
		{"image.png|invalid", "image.png", 0, 0},
		{"image.png|100xinvalid", "image.png", 100, 0},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			target, width, height := parseEmbedDimensions(tt.input)
			if target != tt.wantTarget {
				t.Errorf("target = %q, want %q", target, tt.wantTarget)
			}
			if width != tt.wantWidth {
				t.Errorf("width = %d, want %d", width, tt.wantWidth)
			}
			if height != tt.wantHeight {
				t.Errorf("height = %d, want %d", height, tt.wantHeight)
			}
		})
	}
}

// Nested embed tests.

type mockEmbedResolver struct {
	content map[string][]byte
}

func (m *mockEmbedResolver) ResolveEmbed(target string) ([]byte, bool) {
	content, ok := m.content[target]
	return content, ok
}

func TestResolveNestedEmbeds_Simple(t *testing.T) {
	p := New()

	note := &ParsedNote{
		Path: "main.md",
		Embeds: []Embed{
			{Target: "child.md"},
		},
	}

	resolver := &mockEmbedResolver{
		content: map[string][]byte{
			"child.md": []byte("# Child\n\nNo embeds here."),
		},
	}

	err := p.ResolveNestedEmbeds(note, resolver)
	if err != nil {
		t.Fatalf("ResolveNestedEmbeds() error: %v", err)
	}
}

func TestResolveNestedEmbeds_CircularReference(t *testing.T) {
	p := New()

	note := &ParsedNote{
		Path: "main.md",
		Embeds: []Embed{
			{Target: "main.md"},
		},
	}

	resolver := &mockEmbedResolver{
		content: map[string][]byte{
			"main.md": []byte("# Main\n\n![[main.md]]"),
		},
	}

	err := p.ResolveNestedEmbeds(note, resolver)
	if err == nil {
		t.Error("ResolveNestedEmbeds() should return error for circular reference")
	}

	embedErr, ok := err.(*EmbedError)
	if !ok {
		t.Errorf("error should be EmbedError, got %T", err)
	}

	if !embedErr.IsCircular {
		t.Error("EmbedError.IsCircular should be true")
	}
}

func TestResolveNestedEmbeds_NilResolver(t *testing.T) {
	p := New()

	note := &ParsedNote{
		Path: "main.md",
		Embeds: []Embed{
			{Target: "child.md"},
		},
	}

	err := p.ResolveNestedEmbeds(note, nil)
	if err != nil {
		t.Fatalf("ResolveNestedEmbeds(nil) should not error: %v", err)
	}
}

func TestResolveNestedEmbeds_MediaSkipped(t *testing.T) {
	p := New()

	note := &ParsedNote{
		Path: "main.md",
		Embeds: []Embed{
			{Target: "image.png", IsImage: true},
			{Target: "doc.pdf", IsPDF: true},
		},
	}

	resolver := &mockEmbedResolver{
		content: map[string][]byte{},
	}

	// Should not error even if media files aren't in resolver.
	err := p.ResolveNestedEmbeds(note, resolver)
	if err != nil {
		t.Fatalf("ResolveNestedEmbeds() error: %v", err)
	}
}

// Dataview tests.

func TestExtractDataviewQueries_TABLE(t *testing.T) {
	content := []byte("```dataview\nTABLE file.name, file.size\nFROM #tag\nWHERE status = \"active\"\n```")

	queries := ExtractDataviewQueries(content)
	if len(queries) != 1 {
		t.Fatalf("queries count = %d, want 1", len(queries))
	}

	q := queries[0]
	if q.Type != QueryTypeTable {
		t.Errorf("Type = %v, want TABLE", q.Type)
	}

	if q.IsInline {
		t.Error("IsInline should be false")
	}
}

func TestExtractDataviewQueries_LIST(t *testing.T) {
	content := []byte("```dataview\nLIST\nFROM \"folder\"\n```")

	queries := ExtractDataviewQueries(content)
	if len(queries) != 1 {
		t.Fatalf("queries count = %d, want 1", len(queries))
	}

	if queries[0].Type != QueryTypeList {
		t.Errorf("Type = %v, want LIST", queries[0].Type)
	}
}

func TestExtractDataviewQueries_TASK(t *testing.T) {
	content := []byte("```dataview\nTASK\nFROM #project\n```")

	queries := ExtractDataviewQueries(content)
	if len(queries) != 1 {
		t.Fatalf("queries count = %d, want 1", len(queries))
	}

	if queries[0].Type != QueryTypeTask {
		t.Errorf("Type = %v, want TASK", queries[0].Type)
	}
}

func TestExtractDataviewQueries_Inline(t *testing.T) {
	content := []byte("The file name is `=this.file.name` and size is `=this.file.size`.")

	queries := ExtractDataviewQueries(content)
	if len(queries) != 2 {
		t.Fatalf("queries count = %d, want 2", len(queries))
	}

	for _, q := range queries {
		if !q.IsInline {
			t.Error("IsInline should be true")
		}
		if q.Type != QueryTypeInline {
			t.Errorf("Type = %v, want INLINE", q.Type)
		}
	}
}

func TestContainsDataview(t *testing.T) {
	tests := []struct {
		name    string
		content []byte
		want    bool
	}{
		{"block query", []byte("```dataview\nTABLE file.name\n```"), true},
		{"inline query", []byte("Size: `=this.file.size`"), true},
		{"js query", []byte("```dataviewjs\ndv.list([1])\n```"), true},
		{"no query", []byte("# Regular markdown\n\nNo queries here."), false},
		{"code block", []byte("```python\nprint('hello')\n```"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ContainsDataview(tt.content)
			if got != tt.want {
				t.Errorf("ContainsDataview() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSnapshotPlaceholder(t *testing.T) {
	q := DataviewQuery{
		Raw:      "TABLE file.name\nFROM #tag",
		Type:     QueryTypeTable,
		IsInline: false,
	}

	placeholder := SnapshotPlaceholder(q)
	if placeholder == "" {
		t.Error("placeholder should not be empty")
	}

	// Check it contains the query.
	if len(placeholder) < len(q.Raw) {
		t.Error("placeholder should contain the query text")
	}
}

func TestSnapshotPlaceholder_Inline(t *testing.T) {
	q := DataviewQuery{
		Raw:      "this.file.name",
		Type:     QueryTypeInline,
		IsInline: true,
	}

	placeholder := SnapshotPlaceholder(q)
	if placeholder == "" {
		t.Error("placeholder should not be empty")
	}

	expected := "[dataview: this.file.name]"
	if placeholder != expected {
		t.Errorf("placeholder = %q, want %q", placeholder, expected)
	}
}

// Frontmatter serialization tests.

func TestSerializeFrontmatter(t *testing.T) {
	fm := map[string]any{
		"title": "Test",
		"tags":  []string{"a", "b"},
	}

	data, err := SerializeFrontmatter(fm)
	if err != nil {
		t.Fatalf("SerializeFrontmatter() error: %v", err)
	}

	if len(data) == 0 {
		t.Error("serialized data should not be empty")
	}

	// Should start and end with delimiters.
	s := string(data)
	if s[:3] != "---" {
		t.Error("should start with ---")
	}
}

func TestSerializeFrontmatter_Empty(t *testing.T) {
	fm := map[string]any{}

	data, err := SerializeFrontmatter(fm)
	if err != nil {
		t.Fatalf("SerializeFrontmatter() error: %v", err)
	}

	if data != nil {
		t.Errorf("serialized empty frontmatter should be nil, got %q", string(data))
	}
}

func TestMergeFrontmatter(t *testing.T) {
	existing := map[string]any{
		"title":  "Old Title",
		"author": "John",
	}

	new := map[string]any{
		"title": "New Title",
		"date":  "2024-01-15",
	}

	merged := MergeFrontmatter(existing, new)

	if merged["title"] != "New Title" {
		t.Errorf("title should be overridden, got %v", merged["title"])
	}

	if merged["author"] != "John" {
		t.Errorf("author should be preserved, got %v", merged["author"])
	}

	if merged["date"] != "2024-01-15" {
		t.Errorf("date should be added, got %v", merged["date"])
	}
}
