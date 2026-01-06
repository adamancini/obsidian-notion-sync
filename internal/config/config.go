// Package config handles configuration loading and management for obsidian-notion.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	// DefaultBatchSize is the default maximum blocks per API request.
	DefaultBatchSize = 100

	// MaxBatchSize is the maximum allowed batch size (Notion API limit).
	MaxBatchSize = 100

	// DefaultRequestsPerSecond is the default API rate limit.
	DefaultRequestsPerSecond = 3.0
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

	// Watch contains watch mode configuration.
	Watch WatchConfig `yaml:"watch"`
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

	// DeletionStrategy: "archive", "delete", or "ignore".
	// - archive: Set Notion page archived: true (soft delete, recoverable).
	// - delete: Actually delete from Notion (dangerous, not recoverable).
	// - ignore: Keep in Notion, just remove from sync_state.
	DeletionStrategy string `yaml:"deletion_strategy"`

	// Ignore patterns for files to skip.
	Ignore []string `yaml:"ignore"`
}

// WatchConfig holds watch mode configuration.
type WatchConfig struct {
	// Debounce is the duration to wait after a file change before syncing.
	// Default: 5s. This prevents sync storms during rapid edits.
	Debounce string `yaml:"debounce"`

	// PollInterval is the interval for polling Notion for remote changes.
	// Default: 5m. Set to 0 to disable polling.
	PollInterval string `yaml:"poll_interval"`

	// PIDFile is the path to the PID file for daemon mode.
	// Default: $XDG_RUNTIME_DIR/obsidian-notion.pid or /tmp/obsidian-notion.pid
	PIDFile string `yaml:"pid_file"`

	// LogFile is the path to the log file for daemon mode.
	// Default: stdout
	LogFile string `yaml:"log_file"`
}

// RateLimitConfig holds rate limiting settings.
type RateLimitConfig struct {
	// RequestsPerSecond is the API request rate limit.
	RequestsPerSecond float64 `yaml:"requests_per_second"`

	// BatchSize is the max blocks per API request.
	BatchSize int `yaml:"batch_size"`

	// Workers is the number of parallel workers for processing.
	// Default is 4. Set to 1 for sequential processing.
	Workers int `yaml:"workers"`
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
				"danger":  "🔴",
				"example": "📝",
				"quote":   "💬",
				"success": "✅",
				"failure": "❌",
				"bug":     "🐛",
				"question": "❓",
			},
		},
		Sync: SyncConfig{
			ConflictStrategy: "manual",
			DeletionStrategy: "archive",
			Ignore: []string{
				"templates/**",
				"**/.excalidraw.md",
			},
		},
		RateLimit: RateLimitConfig{
			RequestsPerSecond: DefaultRequestsPerSecond,
			BatchSize:         DefaultBatchSize,
			Workers:           4,
		},
		Watch: WatchConfig{
			Debounce:     "5s",
			PollInterval: "5m",
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

	// Validate conflict strategy if set.
	if c.Sync.ConflictStrategy != "" {
		validStrategies := map[string]bool{
			"local": true, "remote": true, "manual": true, "newer": true,
		}
		if !validStrategies[c.Sync.ConflictStrategy] {
			return fmt.Errorf("invalid conflict_strategy: %s (must be local, remote, manual, or newer)", c.Sync.ConflictStrategy)
		}
	}

	// Validate deletion strategy if set.
	if c.Sync.DeletionStrategy != "" {
		validDeletionStrategies := map[string]bool{
			"archive": true, "delete": true, "ignore": true,
		}
		if !validDeletionStrategies[c.Sync.DeletionStrategy] {
			return fmt.Errorf("invalid deletion_strategy: %s (must be archive, delete, or ignore)", c.Sync.DeletionStrategy)
		}
	}

	// Validate transform settings if set.
	if c.Transform.Dataview != "" {
		validDataview := map[string]bool{"snapshot": true, "placeholder": true}
		if !validDataview[c.Transform.Dataview] {
			return fmt.Errorf("invalid dataview transform: %s (must be snapshot or placeholder)", c.Transform.Dataview)
		}
	}

	if c.Transform.UnresolvedLinks != "" {
		validUnresolved := map[string]bool{"placeholder": true, "text": true, "skip": true}
		if !validUnresolved[c.Transform.UnresolvedLinks] {
			return fmt.Errorf("invalid unresolved_links transform: %s (must be placeholder, text, or skip)", c.Transform.UnresolvedLinks)
		}
	}

	// Validate rate limit settings.
	if c.RateLimit.RequestsPerSecond < 0 {
		return fmt.Errorf("rate_limit.requests_per_second must be non-negative")
	}
	if c.RateLimit.BatchSize < 1 {
		c.RateLimit.BatchSize = DefaultBatchSize
	}
	if c.RateLimit.BatchSize > MaxBatchSize {
		return fmt.Errorf("rate_limit.batch_size must not exceed %d", MaxBatchSize)
	}

	// Validate property mappings in folder mappings.
	for i, mapping := range c.Mappings {
		if mapping.Path == "" {
			return fmt.Errorf("mappings[%d].path is required", i)
		}
		if mapping.Database == "" {
			return fmt.Errorf("mappings[%d].database is required", i)
		}
		for j, prop := range mapping.Properties {
			if prop.Obsidian == "" {
				return fmt.Errorf("mappings[%d].properties[%d].obsidian is required", i, j)
			}
			if prop.Notion == "" {
				return fmt.Errorf("mappings[%d].properties[%d].notion is required", i, j)
			}
			if prop.Type != "" {
				validTypes := map[string]bool{
					"title": true, "rich_text": true, "number": true, "select": true,
					"multi_select": true, "date": true, "checkbox": true, "url": true,
					"email": true, "phone_number": true, "relation": true, "files": true,
				}
				if !validTypes[prop.Type] {
					return fmt.Errorf("mappings[%d].properties[%d].type is invalid: %s", i, j, prop.Type)
				}
			}
		}
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
