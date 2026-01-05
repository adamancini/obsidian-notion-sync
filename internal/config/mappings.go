package config

import (
	"github.com/adamancini/obsidian-notion-sync/internal/transformer"
)

// ToPropertyMappings converts config property mappings to transformer mappings.
func ToPropertyMappings(configs []PropertyMappingConfig) []transformer.PropertyMapping {
	if len(configs) == 0 {
		return transformer.DefaultMappings
	}

	mappings := make([]transformer.PropertyMapping, len(configs))
	for i, cfg := range configs {
		mappings[i] = transformer.PropertyMapping{
			ObsidianKey: cfg.Obsidian,
			NotionName:  cfg.Notion,
			NotionType:  toPropertyType(cfg.Type),
		}
	}

	return mappings
}

// toPropertyType converts a string type to transformer.PropertyType.
func toPropertyType(s string) transformer.PropertyType {
	switch s {
	case "title":
		return transformer.PropertyTypeTitle
	case "rich_text", "text":
		return transformer.PropertyTypeRichText
	case "number":
		return transformer.PropertyTypeNumber
	case "select":
		return transformer.PropertyTypeSelect
	case "multi_select", "tags":
		return transformer.PropertyTypeMultiSelect
	case "date":
		return transformer.PropertyTypeDate
	case "checkbox", "bool":
		return transformer.PropertyTypeCheckbox
	case "url":
		return transformer.PropertyTypeURL
	case "email":
		return transformer.PropertyTypeEmail
	case "phone", "phone_number":
		return transformer.PropertyTypePhone
	default:
		return transformer.PropertyTypeRichText
	}
}

// ToTransformerConfig converts config transform settings to transformer config.
func ToTransformerConfig(cfg TransformConfig) *transformer.Config {
	tc := transformer.DefaultConfig()

	if cfg.Dataview != "" {
		tc.DataviewHandling = cfg.Dataview
	}

	if cfg.UnresolvedLinks != "" {
		tc.UnresolvedLinkStyle = cfg.UnresolvedLinks
	}

	if len(cfg.Callouts) > 0 {
		for k, v := range cfg.Callouts {
			tc.CalloutIcons[k] = v
		}
	}

	return tc
}

// MergePropertyMappings merges folder-specific mappings with defaults.
func MergePropertyMappings(folder []PropertyMappingConfig, defaults []transformer.PropertyMapping) []transformer.PropertyMapping {
	if len(folder) == 0 {
		return defaults
	}

	// Start with defaults.
	result := make([]transformer.PropertyMapping, len(defaults))
	copy(result, defaults)

	// Build map for quick lookup.
	indexMap := make(map[string]int)
	for i, m := range result {
		indexMap[m.ObsidianKey] = i
	}

	// Override/add folder mappings.
	for _, cfg := range folder {
		mapping := transformer.PropertyMapping{
			ObsidianKey: cfg.Obsidian,
			NotionName:  cfg.Notion,
			NotionType:  toPropertyType(cfg.Type),
		}

		if idx, exists := indexMap[cfg.Obsidian]; exists {
			// Override existing mapping.
			result[idx] = mapping
		} else {
			// Add new mapping.
			result = append(result, mapping)
			indexMap[cfg.Obsidian] = len(result) - 1
		}
	}

	return result
}
