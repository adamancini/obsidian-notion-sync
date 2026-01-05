package parser

import (
	"bytes"
	"fmt"

	"gopkg.in/yaml.v3"
)

const (
	frontmatterDelimiter = "---"
)

// extractFrontmatter separates YAML frontmatter from markdown body.
//
// Returns:
//   - frontmatter: parsed YAML as map (empty map if no frontmatter)
//   - body: remaining markdown content after frontmatter
//   - error: if YAML parsing fails
func extractFrontmatter(content []byte) (map[string]any, []byte, error) {
	frontmatter := make(map[string]any)

	// Check for frontmatter delimiter at start.
	if !bytes.HasPrefix(content, []byte(frontmatterDelimiter+"\n")) {
		// No frontmatter, return content as-is.
		return frontmatter, content, nil
	}

	// Find closing delimiter.
	rest := content[4:] // Skip "---\n"
	idx := bytes.Index(rest, []byte("\n"+frontmatterDelimiter+"\n"))
	if idx == -1 {
		// Also check for frontmatter at end of file.
		if bytes.HasSuffix(rest, []byte("\n"+frontmatterDelimiter)) {
			idx = len(rest) - len(frontmatterDelimiter) - 1
		} else {
			// No closing delimiter, treat as no frontmatter.
			return frontmatter, content, nil
		}
	}

	// Extract YAML content.
	yamlContent := rest[:idx]
	body := rest[idx+len(frontmatterDelimiter)+2:] // Skip "\n---\n"

	// Handle case where frontmatter is at end of file.
	if len(body) == 0 && bytes.HasSuffix(rest, []byte("\n"+frontmatterDelimiter)) {
		body = []byte{}
	}

	// Parse YAML.
	if err := yaml.Unmarshal(yamlContent, &frontmatter); err != nil {
		return nil, nil, fmt.Errorf("parse frontmatter: %w", err)
	}

	return frontmatter, body, nil
}

// SerializeFrontmatter converts a map back to YAML frontmatter format.
func SerializeFrontmatter(fm map[string]any) ([]byte, error) {
	if len(fm) == 0 {
		return nil, nil
	}

	var buf bytes.Buffer
	buf.WriteString(frontmatterDelimiter + "\n")

	encoder := yaml.NewEncoder(&buf)
	encoder.SetIndent(2)
	if err := encoder.Encode(fm); err != nil {
		return nil, fmt.Errorf("encode frontmatter: %w", err)
	}
	encoder.Close()

	buf.WriteString(frontmatterDelimiter + "\n")

	return buf.Bytes(), nil
}

// MergeFrontmatter combines existing frontmatter with new values.
// New values take precedence over existing ones.
func MergeFrontmatter(existing, new map[string]any) map[string]any {
	result := make(map[string]any)

	// Copy existing values.
	for k, v := range existing {
		result[k] = v
	}

	// Override with new values.
	for k, v := range new {
		result[k] = v
	}

	return result
}
