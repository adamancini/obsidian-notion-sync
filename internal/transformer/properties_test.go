package transformer

import (
	"testing"
	"time"

	"github.com/jomei/notionapi"
)

func TestNewPropertyMapper_WithDefaultMappings(t *testing.T) {
	pm := NewPropertyMapper(nil)

	if len(pm.mappings) != len(DefaultMappings) {
		t.Errorf("expected %d default mappings, got %d", len(DefaultMappings), len(pm.mappings))
	}

	// Verify default mappings are for title and tags.
	foundTitle := false
	foundTags := false
	for _, m := range pm.mappings {
		if m.ObsidianKey == "title" && m.NotionName == "Name" && m.NotionType == PropertyTypeTitle {
			foundTitle = true
		}
		if m.ObsidianKey == "tags" && m.NotionName == "Tags" && m.NotionType == PropertyTypeMultiSelect {
			foundTags = true
		}
	}

	if !foundTitle {
		t.Error("expected default title mapping")
	}
	if !foundTags {
		t.Error("expected default tags mapping")
	}
}

func TestNewPropertyMapper_WithCustomMappings(t *testing.T) {
	customMappings := []PropertyMapping{
		{ObsidianKey: "status", NotionName: "Status", NotionType: PropertyTypeSelect},
		{ObsidianKey: "due", NotionName: "Due Date", NotionType: PropertyTypeDate},
	}

	pm := NewPropertyMapper(customMappings)

	if len(pm.mappings) != 2 {
		t.Errorf("expected 2 custom mappings, got %d", len(pm.mappings))
	}
}

func TestToNotionProperties_TitleProperty(t *testing.T) {
	mappings := []PropertyMapping{
		{ObsidianKey: "title", NotionName: "Name", NotionType: PropertyTypeTitle},
	}
	pm := NewPropertyMapper(mappings)

	frontmatter := map[string]any{
		"title": "My Note Title",
	}

	props := pm.ToNotionProperties(frontmatter, nil)

	titleProp, ok := props["Name"].(notionapi.TitleProperty)
	if !ok {
		t.Fatal("expected Name to be TitleProperty")
	}

	if len(titleProp.Title) != 1 || titleProp.Title[0].Text.Content != "My Note Title" {
		t.Errorf("expected title 'My Note Title', got %v", titleProp.Title)
	}
}

func TestToNotionProperties_MultiSelectProperty(t *testing.T) {
	mappings := []PropertyMapping{
		{ObsidianKey: "tags", NotionName: "Tags", NotionType: PropertyTypeMultiSelect},
	}
	pm := NewPropertyMapper(mappings)

	frontmatter := map[string]any{}
	tags := []string{"tag1", "tag2", "tag3"}

	props := pm.ToNotionProperties(frontmatter, tags)

	multiSelectProp, ok := props["Tags"].(notionapi.MultiSelectProperty)
	if !ok {
		t.Fatal("expected Tags to be MultiSelectProperty")
	}

	if len(multiSelectProp.MultiSelect) != 3 {
		t.Errorf("expected 3 tags, got %d", len(multiSelectProp.MultiSelect))
	}
}

func TestToNotionProperties_SelectProperty(t *testing.T) {
	mappings := []PropertyMapping{
		{ObsidianKey: "status", NotionName: "Status", NotionType: PropertyTypeSelect},
	}
	pm := NewPropertyMapper(mappings)

	frontmatter := map[string]any{
		"status": "In Progress",
	}

	props := pm.ToNotionProperties(frontmatter, nil)

	selectProp, ok := props["Status"].(notionapi.SelectProperty)
	if !ok {
		t.Fatal("expected Status to be SelectProperty")
	}

	if selectProp.Select.Name != "In Progress" {
		t.Errorf("expected status 'In Progress', got '%s'", selectProp.Select.Name)
	}
}

func TestToNotionProperties_DateProperty(t *testing.T) {
	mappings := []PropertyMapping{
		{ObsidianKey: "due", NotionName: "Due Date", NotionType: PropertyTypeDate},
	}
	pm := NewPropertyMapper(mappings)

	frontmatter := map[string]any{
		"due": "2024-12-25",
	}

	props := pm.ToNotionProperties(frontmatter, nil)

	dateProp, ok := props["Due Date"].(notionapi.DateProperty)
	if !ok {
		t.Fatal("expected Due Date to be DateProperty")
	}

	if dateProp.Date == nil || dateProp.Date.Start == nil {
		t.Fatal("expected date to be set")
	}

	// Check that the date was parsed correctly.
	startTime := time.Time(*dateProp.Date.Start)
	if startTime.Year() != 2024 || startTime.Month() != 12 || startTime.Day() != 25 {
		t.Errorf("expected date 2024-12-25, got %v", startTime)
	}
}

func TestToNotionProperties_NumberProperty(t *testing.T) {
	mappings := []PropertyMapping{
		{ObsidianKey: "priority", NotionName: "Priority", NotionType: PropertyTypeNumber},
	}
	pm := NewPropertyMapper(mappings)

	tests := []struct {
		name     string
		input    any
		expected float64
	}{
		{"int", 42, 42},
		{"float64", 3.14, 3.14},
		{"string int", "100", 100},
		{"string float", "2.5", 2.5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			frontmatter := map[string]any{"priority": tt.input}
			props := pm.ToNotionProperties(frontmatter, nil)

			numProp, ok := props["Priority"].(notionapi.NumberProperty)
			if !ok {
				t.Fatal("expected Priority to be NumberProperty")
			}

			if numProp.Number != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, numProp.Number)
			}
		})
	}
}

func TestToNotionProperties_CheckboxProperty(t *testing.T) {
	mappings := []PropertyMapping{
		{ObsidianKey: "completed", NotionName: "Completed", NotionType: PropertyTypeCheckbox},
	}
	pm := NewPropertyMapper(mappings)

	tests := []struct {
		name     string
		input    any
		expected bool
	}{
		{"bool true", true, true},
		{"bool false", false, false},
		{"string true", "true", true},
		{"string yes", "yes", true},
		{"string 1", "1", true},
		{"string false", "false", false},
		{"int 1", 1, true},
		{"int 0", 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			frontmatter := map[string]any{"completed": tt.input}
			props := pm.ToNotionProperties(frontmatter, nil)

			checkProp, ok := props["Completed"].(notionapi.CheckboxProperty)
			if !ok {
				t.Fatal("expected Completed to be CheckboxProperty")
			}

			if checkProp.Checkbox != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, checkProp.Checkbox)
			}
		})
	}
}

func TestToNotionProperties_URLProperty(t *testing.T) {
	mappings := []PropertyMapping{
		{ObsidianKey: "url", NotionName: "URL", NotionType: PropertyTypeURL},
	}
	pm := NewPropertyMapper(mappings)

	frontmatter := map[string]any{
		"url": "https://example.com",
	}

	props := pm.ToNotionProperties(frontmatter, nil)

	urlProp, ok := props["URL"].(notionapi.URLProperty)
	if !ok {
		t.Fatal("expected URL to be URLProperty")
	}

	if urlProp.URL != "https://example.com" {
		t.Errorf("expected 'https://example.com', got '%s'", urlProp.URL)
	}
}

func TestToNotionProperties_EmailProperty(t *testing.T) {
	mappings := []PropertyMapping{
		{ObsidianKey: "email", NotionName: "Email", NotionType: PropertyTypeEmail},
	}
	pm := NewPropertyMapper(mappings)

	frontmatter := map[string]any{
		"email": "test@example.com",
	}

	props := pm.ToNotionProperties(frontmatter, nil)

	emailProp, ok := props["Email"].(notionapi.EmailProperty)
	if !ok {
		t.Fatal("expected Email to be EmailProperty")
	}

	if emailProp.Email != "test@example.com" {
		t.Errorf("expected 'test@example.com', got '%s'", emailProp.Email)
	}
}

func TestToNotionProperties_PhoneProperty(t *testing.T) {
	mappings := []PropertyMapping{
		{ObsidianKey: "phone", NotionName: "Phone", NotionType: PropertyTypePhone},
	}
	pm := NewPropertyMapper(mappings)

	frontmatter := map[string]any{
		"phone": "+1-555-555-5555",
	}

	props := pm.ToNotionProperties(frontmatter, nil)

	phoneProp, ok := props["Phone"].(notionapi.PhoneNumberProperty)
	if !ok {
		t.Fatal("expected Phone to be PhoneNumberProperty")
	}

	if phoneProp.PhoneNumber != "+1-555-555-5555" {
		t.Errorf("expected '+1-555-555-5555', got '%s'", phoneProp.PhoneNumber)
	}
}

func TestToNotionProperties_RichTextProperty(t *testing.T) {
	mappings := []PropertyMapping{
		{ObsidianKey: "author", NotionName: "Author", NotionType: PropertyTypeRichText},
	}
	pm := NewPropertyMapper(mappings)

	frontmatter := map[string]any{
		"author": "John Doe",
	}

	props := pm.ToNotionProperties(frontmatter, nil)

	rtProp, ok := props["Author"].(notionapi.RichTextProperty)
	if !ok {
		t.Fatal("expected Author to be RichTextProperty")
	}

	if len(rtProp.RichText) != 1 || rtProp.RichText[0].Text.Content != "John Doe" {
		t.Errorf("expected 'John Doe', got %v", rtProp.RichText)
	}
}

func TestToFrontmatter_MappedProperties(t *testing.T) {
	mappings := []PropertyMapping{
		{ObsidianKey: "title", NotionName: "Name", NotionType: PropertyTypeTitle},
		{ObsidianKey: "tags", NotionName: "Tags", NotionType: PropertyTypeMultiSelect},
	}
	pm := NewPropertyMapper(mappings)

	props := notionapi.Properties{
		"Name": &notionapi.TitleProperty{
			Title: []notionapi.RichText{
				{PlainText: "Test Title"},
			},
		},
		"Tags": &notionapi.MultiSelectProperty{
			MultiSelect: []notionapi.Option{
				{Name: "tag1"},
				{Name: "tag2"},
			},
		},
	}

	frontmatter := pm.ToFrontmatter(props)

	if frontmatter["title"] != "Test Title" {
		t.Errorf("expected title 'Test Title', got %v", frontmatter["title"])
	}

	tags, ok := frontmatter["tags"].([]string)
	if !ok || len(tags) != 2 {
		t.Errorf("expected 2 tags, got %v", frontmatter["tags"])
	}
}

func TestToFrontmatter_UnmappedProperties(t *testing.T) {
	// Use default mappings (only title and tags).
	pm := NewPropertyMapper(nil)

	props := notionapi.Properties{
		"Name": &notionapi.TitleProperty{
			Title: []notionapi.RichText{
				{PlainText: "Test Title"},
			},
		},
		"Status": &notionapi.SelectProperty{
			Select: notionapi.Option{Name: "Done"},
		},
		"Priority": &notionapi.NumberProperty{
			Number: 5,
		},
	}

	frontmatter := pm.ToFrontmatter(props)

	// Title should be mapped via explicit mapping.
	if frontmatter["title"] != "Test Title" {
		t.Errorf("expected title 'Test Title', got %v", frontmatter["title"])
	}

	// Status and Priority should be auto-mapped with lowercase names.
	if frontmatter["status"] != "Done" {
		t.Errorf("expected status 'Done', got %v", frontmatter["status"])
	}

	if frontmatter["priority"] != float64(5) {
		t.Errorf("expected priority 5, got %v", frontmatter["priority"])
	}
}

func TestToFrontmatter_AllPropertyTypes(t *testing.T) {
	pm := NewPropertyMapper(nil)

	props := notionapi.Properties{
		"URL": &notionapi.URLProperty{
			URL: "https://example.com",
		},
		"Email": &notionapi.EmailProperty{
			Email: "test@example.com",
		},
		"Phone": &notionapi.PhoneNumberProperty{
			PhoneNumber: "+1-555-555-5555",
		},
		"Completed": &notionapi.CheckboxProperty{
			Checkbox: true,
		},
		"Author": &notionapi.RichTextProperty{
			RichText: []notionapi.RichText{
				{PlainText: "Jane Doe"},
			},
		},
	}

	frontmatter := pm.ToFrontmatter(props)

	if frontmatter["url"] != "https://example.com" {
		t.Errorf("expected url 'https://example.com', got %v", frontmatter["url"])
	}

	if frontmatter["email"] != "test@example.com" {
		t.Errorf("expected email 'test@example.com', got %v", frontmatter["email"])
	}

	if frontmatter["phone"] != "+1-555-555-5555" {
		t.Errorf("expected phone '+1-555-555-5555', got %v", frontmatter["phone"])
	}

	if frontmatter["completed"] != true {
		t.Errorf("expected completed true, got %v", frontmatter["completed"])
	}

	if frontmatter["author"] != "Jane Doe" {
		t.Errorf("expected author 'Jane Doe', got %v", frontmatter["author"])
	}
}

func TestPropertyMappingFromConfig(t *testing.T) {
	tests := []struct {
		name       string
		obsidian   string
		notion     string
		typeStr    string
		expectType PropertyType
	}{
		{"title", "title", "Name", "title", PropertyTypeTitle},
		{"rich_text", "author", "Author", "rich_text", PropertyTypeRichText},
		{"number", "priority", "Priority", "number", PropertyTypeNumber},
		{"select", "status", "Status", "select", PropertyTypeSelect},
		{"multi_select", "tags", "Tags", "multi_select", PropertyTypeMultiSelect},
		{"date", "due", "Due Date", "date", PropertyTypeDate},
		{"checkbox", "completed", "Completed", "checkbox", PropertyTypeCheckbox},
		{"url", "link", "URL", "url", PropertyTypeURL},
		{"email", "contact", "Email", "email", PropertyTypeEmail},
		{"phone_number", "phone", "Phone", "phone_number", PropertyTypePhone},
		{"unknown defaults to rich_text", "custom", "Custom", "unknown", PropertyTypeRichText},
		{"empty defaults to rich_text", "custom", "Custom", "", PropertyTypeRichText},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mapping := PropertyMappingFromConfig(tt.obsidian, tt.notion, tt.typeStr)

			if mapping.ObsidianKey != tt.obsidian {
				t.Errorf("expected ObsidianKey '%s', got '%s'", tt.obsidian, mapping.ObsidianKey)
			}

			if mapping.NotionName != tt.notion {
				t.Errorf("expected NotionName '%s', got '%s'", tt.notion, mapping.NotionName)
			}

			if mapping.NotionType != tt.expectType {
				t.Errorf("expected NotionType '%s', got '%s'", tt.expectType, mapping.NotionType)
			}
		})
	}
}

func TestPropertyTypeFromString(t *testing.T) {
	tests := []struct {
		input    string
		expected PropertyType
	}{
		{"title", PropertyTypeTitle},
		{"rich_text", PropertyTypeRichText},
		{"number", PropertyTypeNumber},
		{"select", PropertyTypeSelect},
		{"multi_select", PropertyTypeMultiSelect},
		{"date", PropertyTypeDate},
		{"checkbox", PropertyTypeCheckbox},
		{"url", PropertyTypeURL},
		{"email", PropertyTypeEmail},
		{"phone_number", PropertyTypePhone},
		{"invalid", PropertyTypeRichText}, // Defaults to rich_text.
		{"", PropertyTypeRichText},        // Empty defaults to rich_text.
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := PropertyTypeFromString(tt.input)
			if result != tt.expected {
				t.Errorf("PropertyTypeFromString(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestToNotionProperties_SkipsMissingFrontmatter(t *testing.T) {
	mappings := []PropertyMapping{
		{ObsidianKey: "title", NotionName: "Name", NotionType: PropertyTypeTitle},
		{ObsidianKey: "status", NotionName: "Status", NotionType: PropertyTypeSelect},
	}
	pm := NewPropertyMapper(mappings)

	// Only provide title, not status.
	frontmatter := map[string]any{
		"title": "Test",
	}

	props := pm.ToNotionProperties(frontmatter, nil)

	// Should have title but not status.
	if _, ok := props["Name"]; !ok {
		t.Error("expected Name property to be set")
	}

	if _, ok := props["Status"]; ok {
		t.Error("expected Status property to NOT be set when frontmatter is missing")
	}
}

func TestDateParsing(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		year    int
		month   int
		day     int
		wantErr bool
	}{
		{"YYYY-MM-DD", "2024-12-25", 2024, 12, 25, false},
		{"YYYY/MM/DD", "2024/12/25", 2024, 12, 25, false},
		{"MM/DD/YYYY", "12/25/2024", 2024, 12, 25, false},
		{"Jan 2, 2006", "Dec 25, 2024", 2024, 12, 25, false},
		{"January 2, 2006", "December 25, 2024", 2024, 12, 25, false},
		{"RFC3339", "2024-12-25T10:30:00Z", 2024, 12, 25, false},
		{"ISO with time", "2024-12-25T10:30:00", 2024, 12, 25, false},
		{"invalid", "not a date", 0, 0, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed, err := parseDate(tt.input)

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error for %q, got none", tt.input)
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error for %q: %v", tt.input, err)
				return
			}

			if parsed.Year() != tt.year || int(parsed.Month()) != tt.month || parsed.Day() != tt.day {
				t.Errorf("parseDate(%q) = %v, want %d-%02d-%02d", tt.input, parsed, tt.year, tt.month, tt.day)
			}
		})
	}
}

func TestRoundTrip_FrontmatterToNotionAndBack(t *testing.T) {
	mappings := []PropertyMapping{
		{ObsidianKey: "title", NotionName: "Name", NotionType: PropertyTypeTitle},
		{ObsidianKey: "tags", NotionName: "Tags", NotionType: PropertyTypeMultiSelect},
		{ObsidianKey: "status", NotionName: "Status", NotionType: PropertyTypeSelect},
		{ObsidianKey: "completed", NotionName: "Completed", NotionType: PropertyTypeCheckbox},
	}
	pm := NewPropertyMapper(mappings)

	original := map[string]any{
		"title":     "My Note",
		"status":    "Done",
		"completed": true,
	}
	originalTags := []string{"tag1", "tag2"}

	// Convert to Notion properties.
	props := pm.ToNotionProperties(original, originalTags)

	// Convert back to frontmatter.
	roundTripped := pm.ToFrontmatter(props)

	// Verify round trip.
	if roundTripped["title"] != original["title"] {
		t.Errorf("title: expected %v, got %v", original["title"], roundTripped["title"])
	}

	if roundTripped["status"] != original["status"] {
		t.Errorf("status: expected %v, got %v", original["status"], roundTripped["status"])
	}

	if roundTripped["completed"] != original["completed"] {
		t.Errorf("completed: expected %v, got %v", original["completed"], roundTripped["completed"])
	}

	tags, ok := roundTripped["tags"].([]string)
	if !ok || len(tags) != 2 {
		t.Errorf("tags: expected %v, got %v", originalTags, roundTripped["tags"])
	}
}
