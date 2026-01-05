package parser

import (
	"bytes"
	"errors"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	frontmatterDelimiter = "---"
)

// FrontmatterError represents an error in frontmatter parsing.
type FrontmatterError struct {
	// Line is the line number where the error occurred (1-indexed).
	Line int

	// Column is the column number where the error occurred (1-indexed).
	Column int

	// Message describes the error.
	Message string

	// Cause is the underlying error, if any.
	Cause error
}

func (e *FrontmatterError) Error() string {
	if e.Line > 0 {
		return fmt.Sprintf("frontmatter error at line %d: %s", e.Line, e.Message)
	}
	return fmt.Sprintf("frontmatter error: %s", e.Message)
}

func (e *FrontmatterError) Unwrap() error {
	return e.Cause
}

// FrontmatterParseMode controls how frontmatter parsing errors are handled.
type FrontmatterParseMode int

const (
	// FrontmatterStrict returns an error on malformed frontmatter.
	FrontmatterStrict FrontmatterParseMode = iota

	// FrontmatterLenient attempts to recover from malformed frontmatter.
	FrontmatterLenient
)

// extractFrontmatter separates YAML frontmatter from markdown body.
//
// Returns:
//   - frontmatter: parsed YAML as map (empty map if no frontmatter)
//   - body: remaining markdown content after frontmatter
//   - error: if YAML parsing fails
func extractFrontmatter(content []byte) (map[string]any, []byte, error) {
	return extractFrontmatterWithMode(content, FrontmatterStrict)
}

// extractFrontmatterWithMode separates YAML frontmatter with configurable error handling.
func extractFrontmatterWithMode(content []byte, mode FrontmatterParseMode) (map[string]any, []byte, error) {
	frontmatter := make(map[string]any)

	// Handle empty content.
	if len(content) == 0 {
		return frontmatter, content, nil
	}

	// Check for frontmatter delimiter at start.
	// Support both "---\n" and "---\r\n" (Windows line endings).
	if !bytes.HasPrefix(content, []byte(frontmatterDelimiter+"\n")) &&
		!bytes.HasPrefix(content, []byte(frontmatterDelimiter+"\r\n")) {
		// No frontmatter, return content as-is.
		return frontmatter, content, nil
	}

	// Determine the actual line ending used.
	lineEnding := "\n"
	skipLen := 4 // len("---\n")
	if bytes.HasPrefix(content, []byte(frontmatterDelimiter+"\r\n")) {
		lineEnding = "\r\n"
		skipLen = 5 // len("---\r\n")
	}

	// Find closing delimiter.
	rest := content[skipLen:]
	closingDelim := lineEnding + frontmatterDelimiter + lineEnding
	idx := bytes.Index(rest, []byte(closingDelim))

	// Handle empty frontmatter: "---\n---\n" where closing delimiter is at start.
	if idx == -1 && bytes.HasPrefix(rest, []byte(frontmatterDelimiter+lineEnding)) {
		// Empty frontmatter block.
		body := rest[len(frontmatterDelimiter+lineEnding):]
		return make(map[string]any), body, nil
	}

	if idx == -1 {
		// Also check for frontmatter at end of file (no trailing newline).
		endDelim := lineEnding + frontmatterDelimiter
		if bytes.HasSuffix(rest, []byte(endDelim)) {
			idx = len(rest) - len(endDelim)
		} else {
			// No closing delimiter found.
			if mode == FrontmatterLenient {
				// In lenient mode, treat entire content as body.
				return frontmatter, content, nil
			}
			return nil, nil, &FrontmatterError{
				Line:    1,
				Message: "unclosed frontmatter block: missing closing '---'",
			}
		}
	}

	// Extract YAML content.
	yamlContent := rest[:idx]

	// Calculate body start position.
	bodyStart := idx + len(closingDelim)
	var body []byte
	if bodyStart <= len(rest) {
		body = rest[bodyStart:]
	} else if bytes.HasSuffix(rest, []byte(lineEnding+frontmatterDelimiter)) {
		// Frontmatter at end of file with no trailing content.
		body = []byte{}
	} else {
		body = []byte{}
	}

	// Handle empty frontmatter block.
	if len(bytes.TrimSpace(yamlContent)) == 0 {
		return frontmatter, body, nil
	}

	// Parse YAML with detailed error handling.
	if err := yaml.Unmarshal(yamlContent, &frontmatter); err != nil {
		var yamlErr *yaml.TypeError
		if errors.As(err, &yamlErr) {
			// Extract line number from YAML error if possible.
			line := extractYAMLErrorLine(yamlErr)
			if mode == FrontmatterLenient {
				// In lenient mode, try to salvage what we can.
				partialFM := attemptPartialYAMLParse(yamlContent)
				return partialFM, body, nil
			}
			return nil, nil, &FrontmatterError{
				Line:    line + 1, // Adjust for frontmatter delimiter line.
				Message: fmt.Sprintf("invalid YAML: %v", err),
				Cause:   err,
			}
		}
		if mode == FrontmatterLenient {
			// In lenient mode, return empty frontmatter and include YAML as part of body.
			return frontmatter, content, nil
		}
		return nil, nil, &FrontmatterError{
			Message: fmt.Sprintf("parse frontmatter: %v", err),
			Cause:   err,
		}
	}

	// Validate frontmatter types (detect common issues).
	if err := validateFrontmatter(frontmatter); err != nil {
		if mode == FrontmatterLenient {
			// In lenient mode, return the parsed frontmatter anyway.
			return frontmatter, body, nil
		}
		return nil, nil, err
	}

	return frontmatter, body, nil
}

// validateFrontmatter checks for common frontmatter issues.
func validateFrontmatter(fm map[string]any) error {
	for key, value := range fm {
		// Check for nil values (can happen with malformed YAML).
		if value == nil {
			continue // nil values are acceptable.
		}

		// Check for circular references or deeply nested structures.
		if err := checkDepth(value, 0, 20); err != nil {
			return &FrontmatterError{
				Message: fmt.Sprintf("invalid value for '%s': %v", key, err),
			}
		}
	}
	return nil
}

// checkDepth ensures values don't exceed maximum nesting depth.
func checkDepth(v any, current, max int) error {
	if current > max {
		return fmt.Errorf("maximum nesting depth exceeded")
	}

	switch val := v.(type) {
	case map[string]any:
		for _, nested := range val {
			if err := checkDepth(nested, current+1, max); err != nil {
				return err
			}
		}
	case []any:
		for _, item := range val {
			if err := checkDepth(item, current+1, max); err != nil {
				return err
			}
		}
	}
	return nil
}

// extractYAMLErrorLine attempts to extract the line number from a YAML error.
func extractYAMLErrorLine(err *yaml.TypeError) int {
	// yaml.TypeError.Errors contains error messages with line info.
	if len(err.Errors) > 0 {
		// Try to parse "line X:" from error message.
		msg := err.Errors[0]
		if idx := strings.Index(msg, "line "); idx >= 0 {
			var line int
			if _, err := fmt.Sscanf(msg[idx:], "line %d:", &line); err == nil {
				return line
			}
		}
	}
	return 0
}

// attemptPartialYAMLParse tries to parse YAML line by line, collecting valid entries.
func attemptPartialYAMLParse(content []byte) map[string]any {
	result := make(map[string]any)
	lines := bytes.Split(content, []byte("\n"))

	var currentKey string
	var multilineValue bytes.Buffer
	inMultiline := false

	for _, line := range lines {
		lineStr := string(line)

		// Skip empty lines and comments.
		trimmed := strings.TrimSpace(lineStr)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		// Check for key: value pattern.
		if !inMultiline && strings.Contains(lineStr, ":") && !strings.HasPrefix(trimmed, "-") {
			parts := strings.SplitN(lineStr, ":", 2)
			if len(parts) == 2 {
				key := strings.TrimSpace(parts[0])
				value := strings.TrimSpace(parts[1])

				// Check for multiline indicators.
				if value == "|" || value == ">" {
					inMultiline = true
					currentKey = key
					multilineValue.Reset()
					continue
				}

				// Try to parse the value.
				var parsed any
				if err := yaml.Unmarshal([]byte(value), &parsed); err == nil {
					result[key] = parsed
				} else {
					// Store as string if parsing fails.
					result[key] = value
				}
			}
		} else if inMultiline {
			// Check if we're still in the multiline block (indented).
			if strings.HasPrefix(lineStr, " ") || strings.HasPrefix(lineStr, "\t") {
				multilineValue.WriteString(strings.TrimPrefix(strings.TrimPrefix(lineStr, " "), "\t"))
				multilineValue.WriteString("\n")
			} else {
				// End of multiline block.
				result[currentKey] = strings.TrimSuffix(multilineValue.String(), "\n")
				inMultiline = false

				// Process this line as a new key.
				if strings.Contains(lineStr, ":") {
					parts := strings.SplitN(lineStr, ":", 2)
					if len(parts) == 2 {
						key := strings.TrimSpace(parts[0])
						value := strings.TrimSpace(parts[1])
						result[key] = value
					}
				}
			}
		}
	}

	// Handle case where file ends in multiline block.
	if inMultiline {
		result[currentKey] = strings.TrimSuffix(multilineValue.String(), "\n")
	}

	return result
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
