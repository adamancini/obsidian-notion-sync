// Package obsidian provides reusable utilities for working with Obsidian vaults.
package obsidian

import (
	"bytes"
	"fmt"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	// FrontmatterDelimiter is the YAML frontmatter delimiter.
	FrontmatterDelimiter = "---"
)

// Frontmatter represents parsed YAML frontmatter from an Obsidian note.
type Frontmatter map[string]any

// ParseFrontmatter extracts YAML frontmatter from markdown content.
// Returns the frontmatter and the remaining body content.
func ParseFrontmatter(content []byte) (Frontmatter, []byte, error) {
	fm := make(Frontmatter)

	// Check for frontmatter delimiter at start.
	if !bytes.HasPrefix(content, []byte(FrontmatterDelimiter+"\n")) {
		return fm, content, nil
	}

	// Find closing delimiter.
	rest := content[4:] // Skip "---\n"
	idx := bytes.Index(rest, []byte("\n"+FrontmatterDelimiter+"\n"))
	if idx == -1 {
		// Check for frontmatter at end of file.
		if bytes.HasSuffix(rest, []byte("\n"+FrontmatterDelimiter)) {
			idx = len(rest) - len(FrontmatterDelimiter) - 1
		} else {
			return fm, content, nil
		}
	}

	// Extract YAML content.
	yamlContent := rest[:idx]
	body := rest[idx+len(FrontmatterDelimiter)+2:]

	// Parse YAML.
	if err := yaml.Unmarshal(yamlContent, &fm); err != nil {
		return nil, nil, fmt.Errorf("parse frontmatter: %w", err)
	}

	return fm, body, nil
}

// SerializeFrontmatter converts frontmatter back to YAML format.
func SerializeFrontmatter(fm Frontmatter) ([]byte, error) {
	if len(fm) == 0 {
		return nil, nil
	}

	var buf bytes.Buffer
	buf.WriteString(FrontmatterDelimiter + "\n")

	encoder := yaml.NewEncoder(&buf)
	encoder.SetIndent(2)
	if err := encoder.Encode(fm); err != nil {
		return nil, fmt.Errorf("encode frontmatter: %w", err)
	}
	encoder.Close()

	buf.WriteString(FrontmatterDelimiter + "\n")

	return buf.Bytes(), nil
}

// GetString retrieves a string value from frontmatter.
func (fm Frontmatter) GetString(key string) string {
	if v, ok := fm[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// GetStringSlice retrieves a string slice from frontmatter.
func (fm Frontmatter) GetStringSlice(key string) []string {
	v, ok := fm[key]
	if !ok {
		return nil
	}

	switch val := v.(type) {
	case []string:
		return val
	case []any:
		result := make([]string, 0, len(val))
		for _, item := range val {
			if s, ok := item.(string); ok {
				result = append(result, s)
			}
		}
		return result
	case string:
		return []string{val}
	default:
		return nil
	}
}

// GetInt retrieves an integer value from frontmatter.
func (fm Frontmatter) GetInt(key string) int {
	if v, ok := fm[key]; ok {
		switch val := v.(type) {
		case int:
			return val
		case float64:
			return int(val)
		}
	}
	return 0
}

// GetBool retrieves a boolean value from frontmatter.
func (fm Frontmatter) GetBool(key string) bool {
	if v, ok := fm[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return false
}

// GetTime retrieves a time value from frontmatter.
func (fm Frontmatter) GetTime(key string) time.Time {
	v, ok := fm[key]
	if !ok {
		return time.Time{}
	}

	switch val := v.(type) {
	case time.Time:
		return val
	case string:
		// Try common date formats.
		formats := []string{
			"2006-01-02",
			"2006/01/02",
			"01/02/2006",
			time.RFC3339,
			"2006-01-02T15:04:05",
		}
		for _, format := range formats {
			if t, err := time.Parse(format, val); err == nil {
				return t
			}
		}
	}

	return time.Time{}
}

// Set sets a frontmatter value.
func (fm Frontmatter) Set(key string, value any) {
	fm[key] = value
}

// Delete removes a frontmatter key.
func (fm Frontmatter) Delete(key string) {
	delete(fm, key)
}

// Has checks if a key exists in frontmatter.
func (fm Frontmatter) Has(key string) bool {
	_, ok := fm[key]
	return ok
}

// Merge merges another frontmatter into this one.
// Values from other take precedence.
func (fm Frontmatter) Merge(other Frontmatter) {
	for k, v := range other {
		fm[k] = v
	}
}

// Clone creates a deep copy of frontmatter.
func (fm Frontmatter) Clone() Frontmatter {
	// Use YAML round-trip for deep copy.
	data, _ := yaml.Marshal(fm)
	result := make(Frontmatter)
	yaml.Unmarshal(data, &result)
	return result
}

// Tags extracts tags from the frontmatter.
// Looks for "tags" field (array or comma-separated string).
func (fm Frontmatter) Tags() []string {
	return fm.GetStringSlice("tags")
}

// Aliases extracts aliases from the frontmatter.
func (fm Frontmatter) Aliases() []string {
	return fm.GetStringSlice("aliases")
}

// Title returns the title, falling back to empty string.
func (fm Frontmatter) Title() string {
	return fm.GetString("title")
}
