// Package config handles configuration loading and management for obsidian-notion.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config represents the complete configuration for obsidian-notion.
type Config struct {
	// Vault is the path to the Obsidian vault directory.
	Vault string `yaml:"vault"`

	// Notion contains Notion API configuration.
	Notion NotionConfig `yaml:"notion"`

	// Mappings define folder-to-database mappings.
	Mappings []FolderMapping `yaml:"mappings"`

	// Transform contains content transformation rules.
	Transform TransformConfig `yaml:"transform"`

	// Sync contains synchronization behavior settings.
	Sync SyncConfig `yaml:"sync"`

	// RateLimit configures API rate limiting.
	RateLimit RateLimitConfig `yaml:"rate_limit"`
}

// NotionConfig holds Notion API credentials and defaults.
type NotionConfig struct {
	// Token is the Notion API integration token.
	// Can be a literal value or ${ENV_VAR} reference.
	Token string `yaml:"token"`

	// DefaultDatabase is the default database ID for notes.
	DefaultDatabase string `yaml:"default_database"`

	// DefaultPage is the default parent page ID (alternative to database).
	DefaultPage string `yaml:"default_page"`
}

// FolderMapping maps an Obsidian folder pattern to a Notion database.
type FolderMapping struct {
	// Path is a glob pattern for matching Obsidian paths.
	Path string `yaml:"path"`

	// Database is the Notion database name or ID.
	Database string `yaml:"database"`

	// Properties defines property mappings for this folder.
	Properties []PropertyMappingConfig `yaml:"properties"`
}

// PropertyMappingConfig defines how a frontmatter field maps to Notion.
type PropertyMappingConfig struct {
	// Obsidian is the frontmatter key name.
	Obsidian string `yaml:"obsidian"`

	// Notion is the Notion property name.
	Notion string `yaml:"notion"`

	// Type is the Notion property type.
	Type string `yaml:"type"`
}

// TransformConfig holds content transformation settings.
type TransformConfig struct {
	// Dataview handling: "snapshot" or "placeholder".
	Dataview string `yaml:"dataview"`

	// Callouts maps Obsidian callout types to emoji icons.
	Callouts map[string]string `yaml:"callouts"`

	// UnresolvedLinks handling: "placeholder", "text", or "skip".
	UnresolvedLinks string `yaml:"unresolved_links"`
}

// SyncConfig holds synchronization behavior settings.
type SyncConfig struct {
	// ConflictStrategy: "local", "remote", "manual", or "newer".
	ConflictStrategy string `yaml:"conflict_strategy"`

	// Ignore patterns for files to skip.
	Ignore []string `yaml:"ignore"`
}

// RateLimitConfig holds rate limiting settings.
type RateLimitConfig struct {
	// RequestsPerSecond is the API request rate limit.
	RequestsPerSecond float64 `yaml:"requests_per_second"`

	// BatchSize is the max blocks per API request.
	BatchSize int `yaml:"batch_size"`
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		Transform: TransformConfig{
			Dataview:        "placeholder",
			UnresolvedLinks: "placeholder",
			Callouts: map[string]string{
				"note":    "💡",
				"warning": "⚠️",
				"tip":     "💡",
				"info":    "ℹ️",
			},
		},
		Sync: SyncConfig{
			ConflictStrategy: "manual",
			Ignore: []string{
				"templates/**",
				"**/.excalidraw.md",
			},
		},
		RateLimit: RateLimitConfig{
			RequestsPerSecond: 3,
			BatchSize:         100,
		},
	}
}

// Load loads configuration from a file or default locations.
func Load(path string) (*Config, error) {
	if path != "" {
		return loadFromFile(path)
	}

	// Try default locations in order.
	locations := []string{
		".obsidian-notion.yaml",
		".obsidian-notion.yml",
	}

	// Add user config directory locations.
	if home, err := os.UserHomeDir(); err == nil {
		locations = append(locations,
			filepath.Join(home, ".config", "obsidian-notion", "config.yaml"),
			filepath.Join(home, ".config", "obsidian-notion", "config.yml"),
		)
	}

	for _, loc := range locations {
		if _, err := os.Stat(loc); err == nil {
			return loadFromFile(loc)
		}
	}

	return nil, fmt.Errorf("no configuration file found (tried: %s)", strings.Join(locations, ", "))
}

// loadFromFile loads configuration from a specific file.
func loadFromFile(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config file: %w", err)
	}

	// Start with defaults.
	cfg := DefaultConfig()

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config file: %w", err)
	}

	// Expand environment variables.
	cfg.expandEnvVars()

	// Expand vault path.
	if strings.HasPrefix(cfg.Vault, "~") {
		if home, err := os.UserHomeDir(); err == nil {
			cfg.Vault = filepath.Join(home, cfg.Vault[1:])
		}
	}

	// Validate configuration.
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return cfg, nil
}

// expandEnvVars expands ${ENV_VAR} references in config values.
func (c *Config) expandEnvVars() {
	c.Notion.Token = expandEnv(c.Notion.Token)
	c.Notion.DefaultDatabase = expandEnv(c.Notion.DefaultDatabase)
	c.Notion.DefaultPage = expandEnv(c.Notion.DefaultPage)
	c.Vault = expandEnv(c.Vault)
}

// expandEnv expands ${VAR} or $VAR references.
func expandEnv(s string) string {
	if strings.HasPrefix(s, "${") && strings.HasSuffix(s, "}") {
		envVar := s[2 : len(s)-1]
		return os.Getenv(envVar)
	}
	if strings.HasPrefix(s, "$") {
		return os.Getenv(s[1:])
	}
	return os.ExpandEnv(s)
}

// Validate checks the configuration for required fields and valid values.
func (c *Config) Validate() error {
	if c.Vault == "" {
		return fmt.Errorf("vault path is required")
	}

	if _, err := os.Stat(c.Vault); os.IsNotExist(err) {
		return fmt.Errorf("vault path does not exist: %s", c.Vault)
	}

	if c.Notion.Token == "" {
		return fmt.Errorf("notion.token is required")
	}

	if c.Notion.DefaultDatabase == "" && c.Notion.DefaultPage == "" && len(c.Mappings) == 0 {
		return fmt.Errorf("at least one of notion.default_database, notion.default_page, or mappings is required")
	}

	return nil
}

// Save writes the configuration to a file.
func (c *Config) Save(path string) error {
	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	// Ensure directory exists.
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write config file: %w", err)
	}

	return nil
}

// GetMapping returns the folder mapping that matches the given path.
func (c *Config) GetMapping(path string) *FolderMapping {
	for i := range c.Mappings {
		matched, _ := filepath.Match(c.Mappings[i].Path, path)
		if matched {
			return &c.Mappings[i]
		}
	}
	return nil
}

// GetDatabaseForPath returns the database ID for a given path.
func (c *Config) GetDatabaseForPath(path string) string {
	mapping := c.GetMapping(path)
	if mapping != nil {
		return mapping.Database
	}
	return c.Notion.DefaultDatabase
}
