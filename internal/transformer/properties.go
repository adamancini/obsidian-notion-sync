package transformer

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jomei/notionapi"
)

// PropertyType represents the Notion property type.
type PropertyType string

const (
	PropertyTypeTitle       PropertyType = "title"
	PropertyTypeRichText    PropertyType = "rich_text"
	PropertyTypeNumber      PropertyType = "number"
	PropertyTypeSelect      PropertyType = "select"
	PropertyTypeMultiSelect PropertyType = "multi_select"
	PropertyTypeDate        PropertyType = "date"
	PropertyTypeCheckbox    PropertyType = "checkbox"
	PropertyTypeURL         PropertyType = "url"
	PropertyTypeEmail       PropertyType = "email"
	PropertyTypePhone       PropertyType = "phone_number"
)

// PropertyMapping defines how an Obsidian frontmatter field maps to Notion.
type PropertyMapping struct {
	// ObsidianKey is the frontmatter key name.
	ObsidianKey string

	// NotionName is the Notion property name.
	NotionName string

	// NotionType is the Notion property type.
	NotionType PropertyType

	// Transform is an optional transformation function.
	Transform func(any) any
}

// DefaultMappings provides sensible defaults for common frontmatter fields.
// These are used when no explicit PropertyMappings are configured.
var DefaultMappings = []PropertyMapping{
	{ObsidianKey: "title", NotionName: "Name", NotionType: PropertyTypeTitle},
	{ObsidianKey: "tags", NotionName: "Tags", NotionType: PropertyTypeMultiSelect},
}

// BuiltInMappings provides sensible defaults for additional common frontmatter fields.
// These can be used as a starting point for custom configurations.
var BuiltInMappings = []PropertyMapping{
	{ObsidianKey: "title", NotionName: "Name", NotionType: PropertyTypeTitle},
	{ObsidianKey: "tags", NotionName: "Tags", NotionType: PropertyTypeMultiSelect},
	{ObsidianKey: "status", NotionName: "Status", NotionType: PropertyTypeSelect},
	{ObsidianKey: "due", NotionName: "Due Date", NotionType: PropertyTypeDate},
	{ObsidianKey: "date", NotionName: "Date", NotionType: PropertyTypeDate},
	{ObsidianKey: "priority", NotionName: "Priority", NotionType: PropertyTypeSelect},
	{ObsidianKey: "created", NotionName: "Created", NotionType: PropertyTypeDate},
	{ObsidianKey: "modified", NotionName: "Modified", NotionType: PropertyTypeDate},
	{ObsidianKey: "url", NotionName: "URL", NotionType: PropertyTypeURL},
	{ObsidianKey: "author", NotionName: "Author", NotionType: PropertyTypeRichText},
}

// PropertyMapper handles conversion between frontmatter and Notion properties.
type PropertyMapper struct {
	mappings []PropertyMapping
}

// NewPropertyMapper creates a new PropertyMapper with the given mappings.
// If nil, uses DefaultMappings.
func NewPropertyMapper(mappings []PropertyMapping) *PropertyMapper {
	if mappings == nil {
		mappings = DefaultMappings
	}
	return &PropertyMapper{mappings: mappings}
}

// ToNotionProperties converts frontmatter to Notion properties.
func (m *PropertyMapper) ToNotionProperties(frontmatter map[string]any, tags []string) notionapi.Properties {
	props := make(notionapi.Properties)

	// Process each mapping.
	for _, mapping := range m.mappings {
		value, exists := frontmatter[mapping.ObsidianKey]
		if !exists {
			// Special handling for tags.
			if mapping.ObsidianKey == "tags" && len(tags) > 0 {
				value = tags
			} else {
				continue
			}
		}

		// Apply transformation if defined.
		if mapping.Transform != nil {
			value = mapping.Transform(value)
		}

		// Convert to Notion property.
		prop := m.convertToProperty(value, mapping.NotionType)
		if prop != nil {
			props[mapping.NotionName] = prop
		}
	}

	return props
}

// ToFrontmatter converts Notion properties to frontmatter.
// It first processes explicitly mapped properties, then adds unmapped properties
// with lowercase Notion property names as frontmatter keys.
func (m *PropertyMapper) ToFrontmatter(props notionapi.Properties) map[string]any {
	frontmatter := make(map[string]any)
	processed := make(map[string]bool)

	// First pass: process explicitly mapped properties.
	for _, mapping := range m.mappings {
		prop, exists := props[mapping.NotionName]
		if !exists {
			continue
		}

		value := m.extractPropertyValue(prop, mapping.NotionType)
		if value != nil {
			frontmatter[mapping.ObsidianKey] = value
		}
		processed[mapping.NotionName] = true
	}

	// Second pass: add unmapped properties with lowercase names.
	for name, prop := range props {
		if processed[name] {
			continue
		}

		// Infer type and extract value.
		value := m.extractPropertyValueAuto(prop)
		if value != nil {
			// Use lowercase of Notion property name as frontmatter key.
			frontmatter[strings.ToLower(name)] = value
		}
	}

	return frontmatter
}

// extractPropertyValueAuto extracts a Go value from a Notion property
// by auto-detecting the property type. Handles both pointer and value types.
func (m *PropertyMapper) extractPropertyValueAuto(prop notionapi.Property) any {
	switch p := prop.(type) {
	// Pointer types (from Notion API responses).
	case *notionapi.TitleProperty:
		if len(p.Title) > 0 {
			return p.Title[0].PlainText
		}
	case *notionapi.RichTextProperty:
		if len(p.RichText) > 0 {
			return p.RichText[0].PlainText
		}
	case *notionapi.NumberProperty:
		return p.Number
	case *notionapi.SelectProperty:
		if p.Select.Name != "" {
			return p.Select.Name
		}
	case *notionapi.MultiSelectProperty:
		var values []string
		for _, opt := range p.MultiSelect {
			values = append(values, opt.Name)
		}
		if len(values) > 0 {
			return values
		}
	case *notionapi.DateProperty:
		if p.Date != nil && p.Date.Start != nil {
			return p.Date.Start.String()
		}
	case *notionapi.CheckboxProperty:
		return p.Checkbox
	case *notionapi.URLProperty:
		if p.URL != "" {
			return p.URL
		}
	case *notionapi.EmailProperty:
		if p.Email != "" {
			return p.Email
		}
	case *notionapi.PhoneNumberProperty:
		if p.PhoneNumber != "" {
			return p.PhoneNumber
		}

	// Value types (from our ToNotionProperties function).
	case notionapi.TitleProperty:
		if len(p.Title) > 0 {
			// Check PlainText first (from API), then Text.Content (locally created).
			if p.Title[0].PlainText != "" {
				return p.Title[0].PlainText
			}
			if p.Title[0].Text != nil {
				return p.Title[0].Text.Content
			}
		}
	case notionapi.RichTextProperty:
		if len(p.RichText) > 0 {
			// Check PlainText first (from API), then Text.Content (locally created).
			if p.RichText[0].PlainText != "" {
				return p.RichText[0].PlainText
			}
			if p.RichText[0].Text != nil {
				return p.RichText[0].Text.Content
			}
		}
	case notionapi.NumberProperty:
		return p.Number
	case notionapi.SelectProperty:
		if p.Select.Name != "" {
			return p.Select.Name
		}
	case notionapi.MultiSelectProperty:
		var values []string
		for _, opt := range p.MultiSelect {
			values = append(values, opt.Name)
		}
		if len(values) > 0 {
			return values
		}
	case notionapi.DateProperty:
		if p.Date != nil && p.Date.Start != nil {
			return p.Date.Start.String()
		}
	case notionapi.CheckboxProperty:
		return p.Checkbox
	case notionapi.URLProperty:
		if p.URL != "" {
			return p.URL
		}
	case notionapi.EmailProperty:
		if p.Email != "" {
			return p.Email
		}
	case notionapi.PhoneNumberProperty:
		if p.PhoneNumber != "" {
			return p.PhoneNumber
		}
	}

	return nil
}

// convertToProperty converts a Go value to a Notion property.
func (m *PropertyMapper) convertToProperty(value any, propType PropertyType) notionapi.Property {
	switch propType {
	case PropertyTypeTitle:
		return m.toTitleProperty(value)
	case PropertyTypeRichText:
		return m.toRichTextProperty(value)
	case PropertyTypeNumber:
		return m.toNumberProperty(value)
	case PropertyTypeSelect:
		return m.toSelectProperty(value)
	case PropertyTypeMultiSelect:
		return m.toMultiSelectProperty(value)
	case PropertyTypeDate:
		return m.toDateProperty(value)
	case PropertyTypeCheckbox:
		return m.toCheckboxProperty(value)
	case PropertyTypeURL:
		return m.toURLProperty(value)
	case PropertyTypeEmail:
		return m.toEmailProperty(value)
	case PropertyTypePhone:
		return m.toPhoneProperty(value)
	default:
		return nil
	}
}

func (m *PropertyMapper) toTitleProperty(value any) notionapi.Property {
	text := toString(value)
	return notionapi.TitleProperty{
		Title: []notionapi.RichText{
			{
				Type: notionapi.ObjectTypeText,
				Text: &notionapi.Text{Content: text},
			},
		},
	}
}

func (m *PropertyMapper) toRichTextProperty(value any) notionapi.Property {
	text := toString(value)
	return notionapi.RichTextProperty{
		RichText: []notionapi.RichText{
			{
				Type: notionapi.ObjectTypeText,
				Text: &notionapi.Text{Content: text},
			},
		},
	}
}

func (m *PropertyMapper) toNumberProperty(value any) notionapi.Property {
	var num float64
	switch v := value.(type) {
	case float64:
		num = v
	case int:
		num = float64(v)
	case string:
		num, _ = strconv.ParseFloat(v, 64)
	}
	return notionapi.NumberProperty{Number: num}
}

func (m *PropertyMapper) toSelectProperty(value any) notionapi.Property {
	text := toString(value)
	return notionapi.SelectProperty{
		Select: notionapi.Option{Name: text},
	}
}

func (m *PropertyMapper) toMultiSelectProperty(value any) notionapi.Property {
	var options []notionapi.Option

	switch v := value.(type) {
	case []string:
		for _, s := range v {
			options = append(options, notionapi.Option{Name: s})
		}
	case []any:
		for _, item := range v {
			if s, ok := item.(string); ok {
				options = append(options, notionapi.Option{Name: s})
			}
		}
	case string:
		// Single value or comma-separated.
		for _, s := range strings.Split(v, ",") {
			s = strings.TrimSpace(s)
			if s != "" {
				options = append(options, notionapi.Option{Name: s})
			}
		}
	}

	return notionapi.MultiSelectProperty{MultiSelect: options}
}

func (m *PropertyMapper) toDateProperty(value any) notionapi.Property {
	var dateStr string

	switch v := value.(type) {
	case string:
		dateStr = v
	case time.Time:
		dateStr = v.Format("2006-01-02")
	default:
		dateStr = fmt.Sprintf("%v", v)
	}

	// Parse the date.
	date, err := parseDate(dateStr)
	if err != nil {
		return nil
	}

	notionDate := notionapi.Date(date)
	return notionapi.DateProperty{
		Date: &notionapi.DateObject{
			Start: &notionDate,
		},
	}
}

func (m *PropertyMapper) toCheckboxProperty(value any) notionapi.Property {
	var checked bool

	switch v := value.(type) {
	case bool:
		checked = v
	case string:
		checked = strings.ToLower(v) == "true" || v == "1" || v == "yes"
	case int:
		checked = v != 0
	}

	return notionapi.CheckboxProperty{Checkbox: checked}
}

func (m *PropertyMapper) toURLProperty(value any) notionapi.Property {
	url := toString(value)
	return notionapi.URLProperty{URL: url}
}

func (m *PropertyMapper) toEmailProperty(value any) notionapi.Property {
	email := toString(value)
	return notionapi.EmailProperty{Email: email}
}

func (m *PropertyMapper) toPhoneProperty(value any) notionapi.Property {
	phone := toString(value)
	return notionapi.PhoneNumberProperty{PhoneNumber: phone}
}

// extractPropertyValue extracts a Go value from a Notion property.
// Handles both pointer types (from Notion API) and value types (from ToNotionProperties).
func (m *PropertyMapper) extractPropertyValue(prop notionapi.Property, propType PropertyType) any {
	switch p := prop.(type) {
	// Pointer types (from Notion API responses).
	case *notionapi.TitleProperty:
		if len(p.Title) > 0 {
			return p.Title[0].PlainText
		}
	case *notionapi.RichTextProperty:
		if len(p.RichText) > 0 {
			return p.RichText[0].PlainText
		}
	case *notionapi.NumberProperty:
		return p.Number
	case *notionapi.SelectProperty:
		return p.Select.Name
	case *notionapi.MultiSelectProperty:
		var values []string
		for _, opt := range p.MultiSelect {
			values = append(values, opt.Name)
		}
		return values
	case *notionapi.DateProperty:
		if p.Date != nil && p.Date.Start != nil {
			return p.Date.Start.String()
		}
	case *notionapi.CheckboxProperty:
		return p.Checkbox
	case *notionapi.URLProperty:
		return p.URL
	case *notionapi.EmailProperty:
		return p.Email
	case *notionapi.PhoneNumberProperty:
		return p.PhoneNumber

	// Value types (from our ToNotionProperties function).
	case notionapi.TitleProperty:
		if len(p.Title) > 0 {
			// Check PlainText first (from API), then Text.Content (locally created).
			if p.Title[0].PlainText != "" {
				return p.Title[0].PlainText
			}
			if p.Title[0].Text != nil {
				return p.Title[0].Text.Content
			}
		}
	case notionapi.RichTextProperty:
		if len(p.RichText) > 0 {
			// Check PlainText first (from API), then Text.Content (locally created).
			if p.RichText[0].PlainText != "" {
				return p.RichText[0].PlainText
			}
			if p.RichText[0].Text != nil {
				return p.RichText[0].Text.Content
			}
		}
	case notionapi.NumberProperty:
		return p.Number
	case notionapi.SelectProperty:
		return p.Select.Name
	case notionapi.MultiSelectProperty:
		var values []string
		for _, opt := range p.MultiSelect {
			values = append(values, opt.Name)
		}
		return values
	case notionapi.DateProperty:
		if p.Date != nil && p.Date.Start != nil {
			return p.Date.Start.String()
		}
	case notionapi.CheckboxProperty:
		return p.Checkbox
	case notionapi.URLProperty:
		return p.URL
	case notionapi.EmailProperty:
		return p.Email
	case notionapi.PhoneNumberProperty:
		return p.PhoneNumber
	}

	return nil
}

// Helper functions

// toString converts any value to a string.
func toString(v any) string {
	if v == nil {
		return ""
	}
	switch val := v.(type) {
	case string:
		return val
	default:
		return fmt.Sprintf("%v", val)
	}
}

// parseDate attempts to parse various date formats.
func parseDate(s string) (time.Time, error) {
	formats := []string{
		"2006-01-02",
		"2006/01/02",
		"01/02/2006",
		"Jan 2, 2006",
		"January 2, 2006",
		time.RFC3339,
		"2006-01-02T15:04:05",
	}

	for _, format := range formats {
		if t, err := time.Parse(format, s); err == nil {
			return t, nil
		}
	}

	return time.Time{}, fmt.Errorf("unable to parse date: %s", s)
}

// PropertyMappingFromConfig converts a config type string to PropertyType.
func PropertyMappingFromConfig(obsidian, notion, typeStr string) PropertyMapping {
	return PropertyMapping{
		ObsidianKey: obsidian,
		NotionName:  notion,
		NotionType:  PropertyTypeFromString(typeStr),
	}
}

// PropertyTypeFromString converts a string to PropertyType.
// Returns PropertyTypeRichText as default for unknown types.
func PropertyTypeFromString(s string) PropertyType {
	switch s {
	case "title":
		return PropertyTypeTitle
	case "rich_text":
		return PropertyTypeRichText
	case "number":
		return PropertyTypeNumber
	case "select":
		return PropertyTypeSelect
	case "multi_select":
		return PropertyTypeMultiSelect
	case "date":
		return PropertyTypeDate
	case "checkbox":
		return PropertyTypeCheckbox
	case "url":
		return PropertyTypeURL
	case "email":
		return PropertyTypeEmail
	case "phone_number":
		return PropertyTypePhone
	default:
		// Default to rich_text for unknown types.
		return PropertyTypeRichText
	}
}
