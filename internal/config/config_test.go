package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.RateLimit.RequestsPerSecond != DefaultRequestsPerSecond {
		t.Errorf("expected RequestsPerSecond=%f, got %f", DefaultRequestsPerSecond, cfg.RateLimit.RequestsPerSecond)
	}

	if cfg.RateLimit.BatchSize != DefaultBatchSize {
		t.Errorf("expected BatchSize=%d, got %d", DefaultBatchSize, cfg.RateLimit.BatchSize)
	}

	if cfg.Sync.ConflictStrategy != "manual" {
		t.Errorf("expected ConflictStrategy=manual, got %s", cfg.Sync.ConflictStrategy)
	}

	if cfg.Transform.Dataview != "placeholder" {
		t.Errorf("expected Dataview=placeholder, got %s", cfg.Transform.Dataview)
	}

	if cfg.Transform.UnresolvedLinks != "placeholder" {
		t.Errorf("expected UnresolvedLinks=placeholder, got %s", cfg.Transform.UnresolvedLinks)
	}

	// Check default callout icons.
	expectedCallouts := map[string]string{
		"note":     "💡",
		"warning":  "⚠️",
		"tip":      "💡",
		"info":     "ℹ️",
		"danger":   "🔴",
		"example":  "📝",
		"quote":    "💬",
		"success":  "✅",
		"failure":  "❌",
		"bug":      "🐛",
		"question": "❓",
	}
	for key, expected := range expectedCallouts {
		if cfg.Transform.Callouts[key] != expected {
			t.Errorf("expected Callouts[%s]=%s, got %s", key, expected, cfg.Transform.Callouts[key])
		}
	}
}

func TestExpandEnv(t *testing.T) {
	// Set a test environment variable.
	os.Setenv("TEST_CONFIG_VAR", "test_value")
	defer os.Unsetenv("TEST_CONFIG_VAR")

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "braced env var",
			input:    "${TEST_CONFIG_VAR}",
			expected: "test_value",
		},
		{
			name:     "unbraced env var",
			input:    "$TEST_CONFIG_VAR",
			expected: "test_value",
		},
		{
			name:     "mixed text with env var",
			input:    "prefix_${TEST_CONFIG_VAR}_suffix",
			expected: "prefix_test_value_suffix",
		},
		{
			name:     "no env var",
			input:    "literal_value",
			expected: "literal_value",
		},
		{
			name:     "unset env var",
			input:    "${UNSET_VAR}",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := expandEnv(tt.input)
			if result != tt.expected {
				t.Errorf("expandEnv(%q) = %q, expected %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestLoadFromFile(t *testing.T) {
	// Create a temporary directory for the vault.
	tmpVault, err := os.MkdirTemp("", "test-vault")
	if err != nil {
		t.Fatalf("failed to create temp vault: %v", err)
	}
	defer os.RemoveAll(tmpVault)

	// Create a temporary config file.
	tmpDir, err := os.MkdirTemp("", "test-config")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	configPath := filepath.Join(tmpDir, "config.yaml")

	// Set up environment variable for token.
	os.Setenv("TEST_NOTION_TOKEN", "secret_token_123")
	defer os.Unsetenv("TEST_NOTION_TOKEN")

	configContent := `
vault: ` + tmpVault + `
notion:
  token: ${TEST_NOTION_TOKEN}
  default_database: db123

mappings:
  - path: "work/*"
    database: workdb
    properties:
      - obsidian: status
        notion: Status
        type: select

transform:
  dataview: snapshot
  unresolved_links: text
  callouts:
    custom: "🎯"

sync:
  conflict_strategy: newer
  ignore:
    - "*.tmp"

rate_limit:
  requests_per_second: 2.5
  batch_size: 50
`

	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// Check vault path.
	if cfg.Vault != tmpVault {
		t.Errorf("Vault = %q, expected %q", cfg.Vault, tmpVault)
	}

	// Check token expansion.
	if cfg.Notion.Token != "secret_token_123" {
		t.Errorf("Notion.Token = %q, expected %q", cfg.Notion.Token, "secret_token_123")
	}

	// Check default database.
	if cfg.Notion.DefaultDatabase != "db123" {
		t.Errorf("Notion.DefaultDatabase = %q, expected %q", cfg.Notion.DefaultDatabase, "db123")
	}

	// Check mappings.
	if len(cfg.Mappings) != 1 {
		t.Errorf("len(Mappings) = %d, expected 1", len(cfg.Mappings))
	}
	if cfg.Mappings[0].Path != "work/*" {
		t.Errorf("Mappings[0].Path = %q, expected %q", cfg.Mappings[0].Path, "work/*")
	}
	if cfg.Mappings[0].Database != "workdb" {
		t.Errorf("Mappings[0].Database = %q, expected %q", cfg.Mappings[0].Database, "workdb")
	}

	// Check transform settings.
	if cfg.Transform.Dataview != "snapshot" {
		t.Errorf("Transform.Dataview = %q, expected %q", cfg.Transform.Dataview, "snapshot")
	}
	if cfg.Transform.UnresolvedLinks != "text" {
		t.Errorf("Transform.UnresolvedLinks = %q, expected %q", cfg.Transform.UnresolvedLinks, "text")
	}
	if cfg.Transform.Callouts["custom"] != "🎯" {
		t.Errorf("Transform.Callouts[custom] = %q, expected %q", cfg.Transform.Callouts["custom"], "🎯")
	}

	// Check sync settings.
	if cfg.Sync.ConflictStrategy != "newer" {
		t.Errorf("Sync.ConflictStrategy = %q, expected %q", cfg.Sync.ConflictStrategy, "newer")
	}

	// Check rate limit settings.
	if cfg.RateLimit.RequestsPerSecond != 2.5 {
		t.Errorf("RateLimit.RequestsPerSecond = %f, expected %f", cfg.RateLimit.RequestsPerSecond, 2.5)
	}
	if cfg.RateLimit.BatchSize != 50 {
		t.Errorf("RateLimit.BatchSize = %d, expected %d", cfg.RateLimit.BatchSize, 50)
	}
}

func TestValidate(t *testing.T) {
	// Create a temporary directory for the vault.
	tmpVault, err := os.MkdirTemp("", "test-vault")
	if err != nil {
		t.Fatalf("failed to create temp vault: %v", err)
	}
	defer os.RemoveAll(tmpVault)

	tests := []struct {
		name      string
		config    *Config
		expectErr bool
		errMsg    string
	}{
		{
			name: "valid config with default database",
			config: &Config{
				Vault: tmpVault,
				Notion: NotionConfig{
					Token:           "token123",
					DefaultDatabase: "db123",
				},
				RateLimit: RateLimitConfig{
					RequestsPerSecond: 3,
					BatchSize:         100,
				},
			},
			expectErr: false,
		},
		{
			name: "valid config with default page",
			config: &Config{
				Vault: tmpVault,
				Notion: NotionConfig{
					Token:       "token123",
					DefaultPage: "page123",
				},
				RateLimit: RateLimitConfig{
					RequestsPerSecond: 3,
					BatchSize:         100,
				},
			},
			expectErr: false,
		},
		{
			name: "valid config with mappings",
			config: &Config{
				Vault: tmpVault,
				Notion: NotionConfig{
					Token: "token123",
				},
				Mappings: []FolderMapping{
					{Path: "work/*", Database: "workdb"},
				},
				RateLimit: RateLimitConfig{
					RequestsPerSecond: 3,
					BatchSize:         100,
				},
			},
			expectErr: false,
		},
		{
			name: "missing vault",
			config: &Config{
				Notion: NotionConfig{
					Token:           "token123",
					DefaultDatabase: "db123",
				},
			},
			expectErr: true,
			errMsg:    "vault path is required",
		},
		{
			name: "vault does not exist",
			config: &Config{
				Vault: "/nonexistent/path",
				Notion: NotionConfig{
					Token:           "token123",
					DefaultDatabase: "db123",
				},
			},
			expectErr: true,
			errMsg:    "vault path does not exist",
		},
		{
			name: "missing token",
			config: &Config{
				Vault: tmpVault,
				Notion: NotionConfig{
					DefaultDatabase: "db123",
				},
			},
			expectErr: true,
			errMsg:    "notion.token is required",
		},
		{
			name: "no database or page or mappings",
			config: &Config{
				Vault: tmpVault,
				Notion: NotionConfig{
					Token: "token123",
				},
			},
			expectErr: true,
			errMsg:    "at least one of notion.default_database, notion.default_page, or mappings is required",
		},
		{
			name: "invalid conflict strategy",
			config: &Config{
				Vault: tmpVault,
				Notion: NotionConfig{
					Token:           "token123",
					DefaultDatabase: "db123",
				},
				Sync: SyncConfig{
					ConflictStrategy: "invalid",
				},
				RateLimit: RateLimitConfig{
					RequestsPerSecond: 3,
					BatchSize:         100,
				},
			},
			expectErr: true,
			errMsg:    "invalid conflict_strategy",
		},
		{
			name: "invalid dataview transform",
			config: &Config{
				Vault: tmpVault,
				Notion: NotionConfig{
					Token:           "token123",
					DefaultDatabase: "db123",
				},
				Transform: TransformConfig{
					Dataview: "invalid",
				},
				RateLimit: RateLimitConfig{
					RequestsPerSecond: 3,
					BatchSize:         100,
				},
			},
			expectErr: true,
			errMsg:    "invalid dataview transform",
		},
		{
			name: "invalid unresolved_links transform",
			config: &Config{
				Vault: tmpVault,
				Notion: NotionConfig{
					Token:           "token123",
					DefaultDatabase: "db123",
				},
				Transform: TransformConfig{
					UnresolvedLinks: "invalid",
				},
				RateLimit: RateLimitConfig{
					RequestsPerSecond: 3,
					BatchSize:         100,
				},
			},
			expectErr: true,
			errMsg:    "invalid unresolved_links transform",
		},
		{
			name: "negative rate limit",
			config: &Config{
				Vault: tmpVault,
				Notion: NotionConfig{
					Token:           "token123",
					DefaultDatabase: "db123",
				},
				RateLimit: RateLimitConfig{
					RequestsPerSecond: -1,
					BatchSize:         100,
				},
			},
			expectErr: true,
			errMsg:    "rate_limit.requests_per_second must be non-negative",
		},
		{
			name: "batch size too large",
			config: &Config{
				Vault: tmpVault,
				Notion: NotionConfig{
					Token:           "token123",
					DefaultDatabase: "db123",
				},
				RateLimit: RateLimitConfig{
					RequestsPerSecond: 3,
					BatchSize:         200,
				},
			},
			expectErr: true,
			errMsg:    "rate_limit.batch_size must not exceed",
		},
		{
			name: "mapping missing path",
			config: &Config{
				Vault: tmpVault,
				Notion: NotionConfig{
					Token: "token123",
				},
				Mappings: []FolderMapping{
					{Database: "db123"},
				},
				RateLimit: RateLimitConfig{
					RequestsPerSecond: 3,
					BatchSize:         100,
				},
			},
			expectErr: true,
			errMsg:    "mappings[0].path is required",
		},
		{
			name: "mapping missing database",
			config: &Config{
				Vault: tmpVault,
				Notion: NotionConfig{
					Token: "token123",
				},
				Mappings: []FolderMapping{
					{Path: "work/*"},
				},
				RateLimit: RateLimitConfig{
					RequestsPerSecond: 3,
					BatchSize:         100,
				},
			},
			expectErr: true,
			errMsg:    "mappings[0].database is required",
		},
		{
			name: "property missing obsidian field",
			config: &Config{
				Vault: tmpVault,
				Notion: NotionConfig{
					Token: "token123",
				},
				Mappings: []FolderMapping{
					{
						Path:     "work/*",
						Database: "db123",
						Properties: []PropertyMappingConfig{
							{Notion: "Status", Type: "select"},
						},
					},
				},
				RateLimit: RateLimitConfig{
					RequestsPerSecond: 3,
					BatchSize:         100,
				},
			},
			expectErr: true,
			errMsg:    "mappings[0].properties[0].obsidian is required",
		},
		{
			name: "property missing notion field",
			config: &Config{
				Vault: tmpVault,
				Notion: NotionConfig{
					Token: "token123",
				},
				Mappings: []FolderMapping{
					{
						Path:     "work/*",
						Database: "db123",
						Properties: []PropertyMappingConfig{
							{Obsidian: "status", Type: "select"},
						},
					},
				},
				RateLimit: RateLimitConfig{
					RequestsPerSecond: 3,
					BatchSize:         100,
				},
			},
			expectErr: true,
			errMsg:    "mappings[0].properties[0].notion is required",
		},
		{
			name: "property invalid type",
			config: &Config{
				Vault: tmpVault,
				Notion: NotionConfig{
					Token: "token123",
				},
				Mappings: []FolderMapping{
					{
						Path:     "work/*",
						Database: "db123",
						Properties: []PropertyMappingConfig{
							{Obsidian: "status", Notion: "Status", Type: "invalid_type"},
						},
					},
				},
				RateLimit: RateLimitConfig{
					RequestsPerSecond: 3,
					BatchSize:         100,
				},
			},
			expectErr: true,
			errMsg:    "mappings[0].properties[0].type is invalid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.expectErr {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.errMsg)
				} else if tt.errMsg != "" && !contains(err.Error(), tt.errMsg) {
					t.Errorf("expected error containing %q, got %q", tt.errMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestGetMapping(t *testing.T) {
	cfg := &Config{
		Mappings: []FolderMapping{
			{Path: "work/*", Database: "workdb"},
			{Path: "personal/*", Database: "personaldb"},
		},
	}

	tests := []struct {
		path         string
		expectedDB   string
		expectNil    bool
	}{
		{path: "work/notes.md", expectedDB: "workdb", expectNil: false},
		{path: "personal/diary.md", expectedDB: "personaldb", expectNil: false},
		{path: "other/file.md", expectNil: true},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			mapping := cfg.GetMapping(tt.path)
			if tt.expectNil {
				if mapping != nil {
					t.Errorf("expected nil mapping for %q, got %+v", tt.path, mapping)
				}
			} else {
				if mapping == nil {
					t.Errorf("expected mapping for %q, got nil", tt.path)
				} else if mapping.Database != tt.expectedDB {
					t.Errorf("expected database %q for %q, got %q", tt.expectedDB, tt.path, mapping.Database)
				}
			}
		})
	}
}

func TestGetDatabaseForPath(t *testing.T) {
	cfg := &Config{
		Notion: NotionConfig{
			DefaultDatabase: "default_db",
		},
		Mappings: []FolderMapping{
			{Path: "work/*", Database: "workdb"},
		},
	}

	tests := []struct {
		path     string
		expected string
	}{
		{path: "work/notes.md", expected: "workdb"},
		{path: "other/file.md", expected: "default_db"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			db := cfg.GetDatabaseForPath(tt.path)
			if db != tt.expected {
				t.Errorf("GetDatabaseForPath(%q) = %q, expected %q", tt.path, db, tt.expected)
			}
		})
	}
}

func TestSaveAndLoad(t *testing.T) {
	// Create a temporary directory for the vault.
	tmpVault, err := os.MkdirTemp("", "test-vault")
	if err != nil {
		t.Fatalf("failed to create temp vault: %v", err)
	}
	defer os.RemoveAll(tmpVault)

	// Create a temporary directory for the config.
	tmpDir, err := os.MkdirTemp("", "test-config")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	configPath := filepath.Join(tmpDir, "subdir", "config.yaml")

	original := &Config{
		Vault: tmpVault,
		Notion: NotionConfig{
			Token:           "test_token",
			DefaultDatabase: "test_db",
		},
		Mappings: []FolderMapping{
			{Path: "work/*", Database: "workdb"},
		},
		Transform: TransformConfig{
			Dataview:        "snapshot",
			UnresolvedLinks: "text",
			Callouts:        map[string]string{"note": "📝"},
		},
		Sync: SyncConfig{
			ConflictStrategy: "newer",
			Ignore:           []string{"*.tmp"},
		},
		RateLimit: RateLimitConfig{
			RequestsPerSecond: 2.5,
			BatchSize:         50,
		},
	}

	// Save the config.
	if err := original.Save(configPath); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// Load it back.
	loaded, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// Compare key fields.
	if loaded.Vault != original.Vault {
		t.Errorf("Vault = %q, expected %q", loaded.Vault, original.Vault)
	}
	if loaded.Notion.Token != original.Notion.Token {
		t.Errorf("Notion.Token = %q, expected %q", loaded.Notion.Token, original.Notion.Token)
	}
	if loaded.Transform.Dataview != original.Transform.Dataview {
		t.Errorf("Transform.Dataview = %q, expected %q", loaded.Transform.Dataview, original.Transform.Dataview)
	}
	if loaded.Sync.ConflictStrategy != original.Sync.ConflictStrategy {
		t.Errorf("Sync.ConflictStrategy = %q, expected %q", loaded.Sync.ConflictStrategy, original.Sync.ConflictStrategy)
	}
}

func TestLoadNoConfigFile(t *testing.T) {
	// Save current working directory.
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get cwd: %v", err)
	}

	// Create a temporary empty directory.
	tmpDir, err := os.MkdirTemp("", "test-no-config")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Change to the temp directory.
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to chdir: %v", err)
	}
	defer os.Chdir(cwd)

	// Try to load without any config file.
	_, err = Load("")
	if err == nil {
		t.Error("expected error when no config file exists, got nil")
	}
}

func TestTildeExpansion(t *testing.T) {
	// Create a temp config file with tilde path.
	tmpDir, err := os.MkdirTemp("", "test-tilde")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Get home directory for comparison.
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("skipping tilde test: %v", err)
	}

	// Create a directory inside home for the test.
	testVaultPath := filepath.Join(home, ".test-vault-tilde")
	if err := os.MkdirAll(testVaultPath, 0755); err != nil {
		t.Fatalf("failed to create test vault: %v", err)
	}
	defer os.RemoveAll(testVaultPath)

	configPath := filepath.Join(tmpDir, "config.yaml")
	configContent := `
vault: ~/.test-vault-tilde
notion:
  token: test_token
  default_database: db123
`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	expected := filepath.Join(home, ".test-vault-tilde")
	if cfg.Vault != expected {
		t.Errorf("Vault = %q, expected %q (tilde expansion)", cfg.Vault, expected)
	}
}

// contains is a helper to check if a string contains a substring.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
