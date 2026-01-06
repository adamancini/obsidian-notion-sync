package obsidian

import (
	"testing"
)

func TestParseWikiLink_Basic(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected *WikiLink
	}{
		{
			name:  "simple link",
			input: "[[Target Note]]",
			expected: &WikiLink{
				Raw:    "[[Target Note]]",
				Target: "Target Note",
			},
		},
		{
			name:  "link with alias",
			input: "[[Target Note|Display Text]]",
			expected: &WikiLink{
				Raw:    "[[Target Note|Display Text]]",
				Target: "Target Note",
				Alias:  "Display Text",
			},
		},
		{
			name:  "link with heading",
			input: "[[Target Note#Section]]",
			expected: &WikiLink{
				Raw:     "[[Target Note#Section]]",
				Target:  "Target Note",
				Heading: "Section",
			},
		},
		{
			name:  "link with block reference",
			input: "[[Target Note^abc123]]",
			expected: &WikiLink{
				Raw:    "[[Target Note^abc123]]",
				Target: "Target Note",
				Block:  "abc123",
			},
		},
		{
			name:  "link with heading and alias",
			input: "[[Target Note#Section|Custom Text]]",
			expected: &WikiLink{
				Raw:     "[[Target Note#Section|Custom Text]]",
				Target:  "Target Note",
				Heading: "Section",
				Alias:   "Custom Text",
			},
		},
		{
			name:  "link with heading block and alias",
			input: "[[Target Note#Section^abc|Custom Text]]",
			expected: &WikiLink{
				Raw:     "[[Target Note#Section^abc|Custom Text]]",
				Target:  "Target Note",
				Heading: "Section",
				Block:   "abc",
				Alias:   "Custom Text",
			},
		},
		{
			name:  "link with path",
			input: "[[folder/subfolder/Note]]",
			expected: &WikiLink{
				Raw:    "[[folder/subfolder/Note]]",
				Target: "folder/subfolder/Note",
			},
		},
		{
			name:  "embed",
			input: "![[Image.png]]",
			expected: &WikiLink{
				Raw:     "![[Image.png]]",
				Target:  "Image.png",
				IsEmbed: true,
			},
		},
		{
			name:  "embed with alias",
			input: "![[Image.png|500]]",
			expected: &WikiLink{
				Raw:     "![[Image.png|500]]",
				Target:  "Image.png",
				Alias:   "500",
				IsEmbed: true,
			},
		},
		{
			name:     "invalid input",
			input:    "not a wiki link",
			expected: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := ParseWikiLink(tc.input)

			if tc.expected == nil {
				if result != nil {
					t.Errorf("expected nil, got %+v", result)
				}
				return
			}

			if result == nil {
				t.Fatalf("expected %+v, got nil", tc.expected)
			}

			if result.Raw != tc.expected.Raw {
				t.Errorf("Raw: expected %q, got %q", tc.expected.Raw, result.Raw)
			}
			if result.Target != tc.expected.Target {
				t.Errorf("Target: expected %q, got %q", tc.expected.Target, result.Target)
			}
			if result.Alias != tc.expected.Alias {
				t.Errorf("Alias: expected %q, got %q", tc.expected.Alias, result.Alias)
			}
			if result.Heading != tc.expected.Heading {
				t.Errorf("Heading: expected %q, got %q", tc.expected.Heading, result.Heading)
			}
			if result.Block != tc.expected.Block {
				t.Errorf("Block: expected %q, got %q", tc.expected.Block, result.Block)
			}
			if result.IsEmbed != tc.expected.IsEmbed {
				t.Errorf("IsEmbed: expected %v, got %v", tc.expected.IsEmbed, result.IsEmbed)
			}
		})
	}
}

func TestExtractWikiLinks(t *testing.T) {
	content := []byte(`# My Note

This has a [[Simple Link]] and also [[Another Note|with alias]].

There's also a [[Path/To/Note#Heading]] reference.

And multiple on one line: [[One]] and [[Two]] and [[Three]].
`)

	links := ExtractWikiLinks(content)

	expectedTargets := []string{
		"Simple Link",
		"Another Note",
		"Path/To/Note",
		"One",
		"Two",
		"Three",
	}

	if len(links) != len(expectedTargets) {
		t.Fatalf("expected %d links, got %d", len(expectedTargets), len(links))
	}

	for i, target := range expectedTargets {
		if links[i].Target != target {
			t.Errorf("link %d: expected target %q, got %q", i, target, links[i].Target)
		}
	}

	// Check alias on second link
	if links[1].Alias != "with alias" {
		t.Errorf("expected alias 'with alias', got %q", links[1].Alias)
	}

	// Check heading on third link
	if links[2].Heading != "Heading" {
		t.Errorf("expected heading 'Heading', got %q", links[2].Heading)
	}
}

func TestExtractEmbeds(t *testing.T) {
	content := []byte(`# My Note

Here's an image: ![[photo.png]]

And a PDF: ![[document.pdf|100%]]

And a note embed: ![[Other Note]]
`)

	embeds := ExtractEmbeds(content)

	if len(embeds) != 3 {
		t.Fatalf("expected 3 embeds, got %d", len(embeds))
	}

	// All should be embeds
	for i, embed := range embeds {
		if !embed.IsEmbed {
			t.Errorf("embed %d: expected IsEmbed=true", i)
		}
	}

	// Check targets
	expectedTargets := []string{"photo.png", "document.pdf", "Other Note"}
	for i, target := range expectedTargets {
		if embeds[i].Target != target {
			t.Errorf("embed %d: expected target %q, got %q", i, target, embeds[i].Target)
		}
	}

	// Check alias on PDF embed
	if embeds[1].Alias != "100%" {
		t.Errorf("expected alias '100%%', got %q", embeds[1].Alias)
	}
}

func TestWikiLink_Display(t *testing.T) {
	tests := []struct {
		name     string
		link     WikiLink
		expected string
	}{
		{
			name:     "simple link",
			link:     WikiLink{Target: "My Note"},
			expected: "My Note",
		},
		{
			name:     "link with alias",
			link:     WikiLink{Target: "My Note", Alias: "Custom Display"},
			expected: "Custom Display",
		},
		{
			name:     "link with path",
			link:     WikiLink{Target: "folder/subfolder/Note"},
			expected: "Note",
		},
		{
			name:     "link with .md extension",
			link:     WikiLink{Target: "My Note.md"},
			expected: "My Note",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.link.Display()
			if result != tc.expected {
				t.Errorf("expected %q, got %q", tc.expected, result)
			}
		})
	}
}

func TestWikiLink_NormalizedTarget(t *testing.T) {
	tests := []struct {
		name     string
		link     WikiLink
		expected string
	}{
		{
			name:     "simple target",
			link:     WikiLink{Target: "My Note"},
			expected: "My Note",
		},
		{
			name:     "target with .md extension",
			link:     WikiLink{Target: "My Note.md"},
			expected: "My Note",
		},
		{
			name:     "target with path",
			link:     WikiLink{Target: "folder/Note.md"},
			expected: "folder/Note",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.link.NormalizedTarget()
			if result != tc.expected {
				t.Errorf("expected %q, got %q", tc.expected, result)
			}
		})
	}
}

func TestWikiLink_String(t *testing.T) {
	tests := []struct {
		name     string
		link     WikiLink
		expected string
	}{
		{
			name:     "simple link",
			link:     WikiLink{Target: "My Note"},
			expected: "[[My Note]]",
		},
		{
			name:     "link with heading",
			link:     WikiLink{Target: "My Note", Heading: "Section"},
			expected: "[[My Note#Section]]",
		},
		{
			name:     "link with block",
			link:     WikiLink{Target: "My Note", Block: "abc123"},
			expected: "[[My Note^abc123]]",
		},
		{
			name:     "link with alias",
			link:     WikiLink{Target: "My Note", Alias: "Display"},
			expected: "[[My Note|Display]]",
		},
		{
			name:     "link with all parts",
			link:     WikiLink{Target: "My Note", Heading: "Sec", Block: "blk", Alias: "Disp"},
			expected: "[[My Note#Sec^blk|Disp]]",
		},
		{
			name:     "embed",
			link:     WikiLink{Target: "image.png", IsEmbed: true},
			expected: "![[image.png]]",
		},
		{
			name:     "embed with alias",
			link:     WikiLink{Target: "image.png", Alias: "500", IsEmbed: true},
			expected: "![[image.png|500]]",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.link.String()
			if result != tc.expected {
				t.Errorf("expected %q, got %q", tc.expected, result)
			}
		})
	}
}

func TestReplaceWikiLinks(t *testing.T) {
	content := []byte("See [[Note A]] and [[Note B|alias]] for details.")

	// Replace with Markdown links
	result := ReplaceWikiLinks(content, func(link WikiLink) string {
		display := link.Display()
		return "[" + display + "](notion://page/" + link.Target + ")"
	})

	expected := "See [Note A](notion://page/Note A) and [alias](notion://page/Note B) for details."
	if string(result) != expected {
		t.Errorf("expected %q, got %q", expected, string(result))
	}
}

func TestReplaceEmbeds(t *testing.T) {
	content := []byte("Image: ![[photo.png|400]]\n\nNote: ![[Embedded Note]]")

	// Replace with HTML
	result := ReplaceEmbeds(content, func(link WikiLink) string {
		if link.IsImageEmbed() {
			return "<img src=\"" + link.Target + "\" width=\"" + link.Alias + "\">"
		}
		return "[Embedded: " + link.Target + "]"
	})

	expected := "Image: <img src=\"photo.png\" width=\"400\">\n\nNote: [Embedded: Embedded Note]"
	if string(result) != expected {
		t.Errorf("expected %q, got %q", expected, string(result))
	}
}

func TestWikiLink_IsImageEmbed(t *testing.T) {
	tests := []struct {
		name     string
		link     WikiLink
		expected bool
	}{
		{
			name:     "png image",
			link:     WikiLink{Target: "photo.png", IsEmbed: true},
			expected: true,
		},
		{
			name:     "jpg image",
			link:     WikiLink{Target: "photo.jpg", IsEmbed: true},
			expected: true,
		},
		{
			name:     "jpeg image",
			link:     WikiLink{Target: "photo.jpeg", IsEmbed: true},
			expected: true,
		},
		{
			name:     "gif image",
			link:     WikiLink{Target: "animation.gif", IsEmbed: true},
			expected: true,
		},
		{
			name:     "svg image",
			link:     WikiLink{Target: "diagram.svg", IsEmbed: true},
			expected: true,
		},
		{
			name:     "webp image",
			link:     WikiLink{Target: "photo.webp", IsEmbed: true},
			expected: true,
		},
		{
			name:     "case insensitive",
			link:     WikiLink{Target: "PHOTO.PNG", IsEmbed: true},
			expected: true,
		},
		{
			name:     "pdf is not image",
			link:     WikiLink{Target: "document.pdf", IsEmbed: true},
			expected: false,
		},
		{
			name:     "note is not image",
			link:     WikiLink{Target: "My Note", IsEmbed: true},
			expected: false,
		},
		{
			name:     "non-embed link",
			link:     WikiLink{Target: "photo.png", IsEmbed: false},
			expected: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.link.IsImageEmbed()
			if result != tc.expected {
				t.Errorf("expected %v, got %v", tc.expected, result)
			}
		})
	}
}

func TestWikiLink_IsFileEmbed(t *testing.T) {
	tests := []struct {
		name     string
		link     WikiLink
		expected bool
	}{
		{
			name:     "pdf file",
			link:     WikiLink{Target: "document.pdf", IsEmbed: true},
			expected: true,
		},
		{
			name:     "audio file",
			link:     WikiLink{Target: "audio.mp3", IsEmbed: true},
			expected: true,
		},
		{
			name:     "image is not file embed",
			link:     WikiLink{Target: "photo.png", IsEmbed: true},
			expected: false,
		},
		{
			name:     "note is not file embed",
			link:     WikiLink{Target: "My Note", IsEmbed: true},
			expected: false,
		},
		{
			name:     "non-embed link",
			link:     WikiLink{Target: "document.pdf", IsEmbed: false},
			expected: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.link.IsFileEmbed()
			if result != tc.expected {
				t.Errorf("expected %v, got %v", tc.expected, result)
			}
		})
	}
}

func TestWikiLink_IsNoteEmbed(t *testing.T) {
	tests := []struct {
		name     string
		link     WikiLink
		expected bool
	}{
		{
			name:     "note without extension",
			link:     WikiLink{Target: "My Note", IsEmbed: true},
			expected: true,
		},
		{
			name:     "note with .md extension",
			link:     WikiLink{Target: "My Note.md", IsEmbed: true},
			expected: true,
		},
		{
			name:     "image is not note embed",
			link:     WikiLink{Target: "photo.png", IsEmbed: true},
			expected: false,
		},
		{
			name:     "pdf is not note embed",
			link:     WikiLink{Target: "document.pdf", IsEmbed: true},
			expected: false,
		},
		{
			name:     "non-embed link",
			link:     WikiLink{Target: "My Note", IsEmbed: false},
			expected: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.link.IsNoteEmbed()
			if result != tc.expected {
				t.Errorf("expected %v, got %v", tc.expected, result)
			}
		})
	}
}

func TestExtractWikiLinks_EmptyContent(t *testing.T) {
	content := []byte("")
	links := ExtractWikiLinks(content)

	if len(links) != 0 {
		t.Errorf("expected 0 links, got %d", len(links))
	}
}

func TestExtractWikiLinks_NoLinks(t *testing.T) {
	content := []byte("This is just plain text without any links.")
	links := ExtractWikiLinks(content)

	if len(links) != 0 {
		t.Errorf("expected 0 links, got %d", len(links))
	}
}

func TestExtractEmbeds_EmptyContent(t *testing.T) {
	content := []byte("")
	embeds := ExtractEmbeds(content)

	if len(embeds) != 0 {
		t.Errorf("expected 0 embeds, got %d", len(embeds))
	}
}
