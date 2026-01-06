package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jomei/notionapi"
	"github.com/spf13/cobra"

	"github.com/adamancini/obsidian-notion-sync/internal/config"
	"github.com/adamancini/obsidian-notion-sync/internal/state"
)

// =============================================================================
// Pure Function Tests
// =============================================================================

func TestLooksLikeUUID(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		// Valid UUIDs
		{"standard UUID with dashes", "550e8400-e29b-41d4-a716-446655440000", true},
		{"UUID without dashes", "550e8400e29b41d4a716446655440000", true},
		{"uppercase UUID", "550E8400-E29B-41D4-A716-446655440000", true},
		{"mixed case UUID", "550e8400-E29B-41d4-A716-446655440000", true},
		{"notion page ID format", "a1b2c3d4e5f67890a1b2c3d4e5f67890", true},

		// Invalid UUIDs
		{"too short", "550e8400-e29b-41d4-a716-44665544", false},
		{"too long", "550e8400-e29b-41d4-a716-4466554400001", false},
		{"invalid characters", "550e8400-e29b-41d4-a716-44665544000g", false},
		{"human readable text", "my-database-name", false},
		{"empty string", "", false},
		{"special characters", "550e8400!e29b-41d4-a716-446655440000", false},
		{"spaces", "550e8400 e29b 41d4 a716 446655440000", false},
		{"partial UUID", "550e8400", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := looksLikeUUID(tc.input)
			if got != tc.want {
				t.Errorf("looksLikeUUID(%q) = %v; want %v", tc.input, got, tc.want)
			}
		})
	}
}

func TestScoreToLabel(t *testing.T) {
	tests := []struct {
		score state.MatchScore
		want  string
	}{
		{state.MatchExact, "exact"},
		{state.MatchCaseInsensitive, "case-insensitive"},
		{state.MatchPrefix, "prefix"},
		{state.MatchFuzzy, "fuzzy"},
		{state.MatchNone, "none"},
		{state.MatchScore(99), "none"}, // Unknown score should default to "none"
	}

	for _, tc := range tests {
		t.Run(tc.want, func(t *testing.T) {
			got := scoreToLabel(tc.score)
			if got != tc.want {
				t.Errorf("scoreToLabel(%d) = %q; want %q", tc.score, got, tc.want)
			}
		})
	}
}

func TestPrintStatusLine(t *testing.T) {
	tests := []struct {
		name     string
		label    string
		count    int
		wantNote string // "note" or "notes"
	}{
		{"zero count", "New (push)", 0, "notes"},
		{"single count", "Modified", 1, "note"},
		{"multiple count", "Synced", 5, "notes"},
		{"large count", "Total", 1000, "notes"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Capture output.
			var buf bytes.Buffer
			old := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			printStatusLine(tc.label, tc.count)

			w.Close()
			os.Stdout = old
			buf.ReadFrom(r)
			output := buf.String()

			// Check that output contains the expected note/notes word.
			if tc.wantNote == "note" {
				// Check for singular, but avoid matching "notes".
				if !containsWord(output, "note") || containsWord(output, "notes") {
					t.Errorf("printStatusLine(%q, %d) output = %q; want singular 'note'", tc.label, tc.count, output)
				}
			} else {
				if !containsWord(output, "notes") {
					t.Errorf("printStatusLine(%q, %d) output = %q; want plural 'notes'", tc.label, tc.count, output)
				}
			}
		})
	}
}

// containsWord checks if output contains the word, with word boundary awareness.
func containsWord(output, word string) bool {
	// Simple check for presence of word.
	return bytes.Contains([]byte(output), []byte(word))
}

func TestSanitizeFilename(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"normal title", "My Note", "My Note"},
		{"slash", "Work/Project", "Work-Project"},
		{"backslash", "Work\\Project", "Work-Project"},
		{"colon", "Meeting: Notes", "Meeting- Notes"},
		{"asterisk", "Star*Wars", "Star-Wars"},
		{"question mark", "What?", "What-"},
		{"double quotes", "\"Quoted\"", "-Quoted-"},
		{"angle brackets", "<tag>", "-tag-"},
		{"pipe", "A|B", "A-B"},
		{"multiple invalid", "A/B\\C:D*E?F\"G<H>I|J", "A-B-C-D-E-F-G-H-I-J"},
		{"leading spaces", "  Title", "Title"},
		{"trailing spaces", "Title  ", "Title"},
		{"leading dots", "...Hidden", "Hidden"},
		{"trailing dots", "File...", "File"},
		{"empty after sanitization", "...", "Untitled"},
		{"all invalid", "/\\:*?\"<>|", "---------"}, // Each invalid char becomes a dash
		{"empty string", "", "Untitled"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := sanitizeFilename(tc.input)
			if got != tc.want {
				t.Errorf("sanitizeFilename(%q) = %q; want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestIsNotFoundError(t *testing.T) {
	tests := []struct {
		name    string
		errStr  string
		want    bool
	}{
		{"nil error", "", false},
		{"not found lowercase", "page not found", true},
		{"could not find", "Could not find page", true},
		{"404 error", "request failed with status 404", true},
		{"other error", "rate limit exceeded", false},
		{"network error", "connection timeout", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var err error
			if tc.errStr != "" {
				err = &testError{msg: tc.errStr}
			}
			got := isNotFoundError(err)
			if got != tc.want {
				t.Errorf("isNotFoundError(%v) = %v; want %v", err, got, tc.want)
			}
		})
	}
}

// testError is a simple error implementation for testing.
type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}

// =============================================================================
// Path Expansion Tests
// =============================================================================

func TestExpandAndValidateVaultPath(t *testing.T) {
	// Create a temporary directory for testing.
	tmpDir, err := os.MkdirTemp("", "vault-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a test file (not a directory).
	testFile := filepath.Join(tmpDir, "testfile.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	tests := []struct {
		name    string
		path    string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid directory",
			path:    tmpDir,
			wantErr: false,
		},
		{
			name:    "non-existent path",
			path:    "/nonexistent/path/that/does/not/exist",
			wantErr: true,
			errMsg:  "does not exist",
		},
		{
			name:    "file instead of directory",
			path:    testFile,
			wantErr: true,
			errMsg:  "not a directory",
		},
		{
			name:    "relative path becomes absolute",
			path:    ".",
			wantErr: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := expandAndValidateVaultPath(tc.path)

			if tc.wantErr {
				if err == nil {
					t.Errorf("expandAndValidateVaultPath(%q) expected error containing %q, got nil", tc.path, tc.errMsg)
				} else if tc.errMsg != "" && !containsWord(err.Error(), tc.errMsg) {
					t.Errorf("expandAndValidateVaultPath(%q) error = %q; want error containing %q", tc.path, err.Error(), tc.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("expandAndValidateVaultPath(%q) unexpected error: %v", tc.path, err)
				}
				// Result should be an absolute path.
				if result != "" && !filepath.IsAbs(result) {
					t.Errorf("expandAndValidateVaultPath(%q) = %q; want absolute path", tc.path, result)
				}
			}
		})
	}
}

func TestExpandAndValidateVaultPath_TildeExpansion(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("Could not determine home directory")
	}

	// Test with ~ prefix - just verify no error and starts with home dir.
	// Note: This test requires home directory to exist.
	result, err := expandAndValidateVaultPath("~")
	if err != nil {
		t.Errorf("expandAndValidateVaultPath(\"~\") unexpected error: %v", err)
	}
	if result != home {
		t.Errorf("expandAndValidateVaultPath(\"~\") = %q; want %q", result, home)
	}
}

// =============================================================================
// getConfig() Tests
// =============================================================================

func TestGetConfig_NilConfig(t *testing.T) {
	// Save original cfg and restore after test.
	originalCfg := cfg
	defer func() { cfg = originalCfg }()

	cfg = nil

	result, err := getConfig()
	if err == nil {
		t.Error("getConfig() expected error when cfg is nil, got nil")
	}
	if err != ErrNoConfig {
		t.Errorf("getConfig() error = %v; want %v", err, ErrNoConfig)
	}
	if result != nil {
		t.Errorf("getConfig() result = %v; want nil", result)
	}
}

func TestGetConfig_SetConfig(t *testing.T) {
	// Save original cfg and restore after test.
	originalCfg := cfg
	defer func() { cfg = originalCfg }()

	testConfig := &config.Config{
		Vault: "/test/vault",
	}
	cfg = testConfig

	result, err := getConfig()
	if err != nil {
		t.Errorf("getConfig() unexpected error: %v", err)
	}
	if result != testConfig {
		t.Errorf("getConfig() result = %v; want %v", result, testConfig)
	}
}

// =============================================================================
// SetVersion Tests
// =============================================================================

func TestSetVersion(t *testing.T) {
	// Save original values.
	origVersion := version
	origCommit := commit
	origDate := date
	defer func() {
		version = origVersion
		commit = origCommit
		date = origDate
	}()

	SetVersion("1.2.3", "abc123", "2024-01-15")

	if version != "1.2.3" {
		t.Errorf("version = %q; want %q", version, "1.2.3")
	}
	if commit != "abc123" {
		t.Errorf("commit = %q; want %q", commit, "abc123")
	}
	if date != "2024-01-15" {
		t.Errorf("date = %q; want %q", date, "2024-01-15")
	}
}

// =============================================================================
// Helper Function Tests
// =============================================================================

func TestFilterByPath(t *testing.T) {
	files := []pushFile{
		{path: "work/project/notes.md"},
		{path: "work/meeting.md"},
		{path: "personal/journal.md"},
		{path: "archive/old.md"},
	}

	tests := []struct {
		name    string
		pattern string
		want    int // Expected number of matches
	}{
		{"match all work files", "work/*.md", 1}, // Only work/meeting.md matches work/*.md
		{"match personal", "personal/*.md", 1},
		{"match all md", "*.md", 0}, // No direct matches at root level
		{"match none", "nonexistent/*.md", 0},
		{"match archive", "archive/*.md", 1},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := filterByPath(files, tc.pattern)
			if len(got) != tc.want {
				t.Errorf("filterByPath(pattern=%q) returned %d files; want %d", tc.pattern, len(got), tc.want)
			}
		})
	}
}

func TestCheckConflicts(t *testing.T) {
	tests := []struct {
		name  string
		files []pushFile
		want  int // Expected number of conflicts
	}{
		{
			name:  "no files",
			files: nil,
			want:  0,
		},
		{
			name: "no conflicts",
			files: []pushFile{
				{path: "a.md", state: &state.SyncState{Status: "synced"}},
				{path: "b.md", state: &state.SyncState{Status: "pending"}},
				{path: "c.md", state: nil},
			},
			want: 0,
		},
		{
			name: "one conflict",
			files: []pushFile{
				{path: "a.md", state: &state.SyncState{Status: "synced"}},
				{path: "b.md", state: &state.SyncState{Status: "conflict"}},
				{path: "c.md", state: &state.SyncState{Status: "pending"}},
			},
			want: 1,
		},
		{
			name: "multiple conflicts",
			files: []pushFile{
				{path: "a.md", state: &state.SyncState{Status: "conflict"}},
				{path: "b.md", state: &state.SyncState{Status: "conflict"}},
				{path: "c.md", state: &state.SyncState{Status: "conflict"}},
			},
			want: 3,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := checkConflicts(tc.files)
			if len(got) != tc.want {
				t.Errorf("checkConflicts() returned %d conflicts; want %d", len(got), tc.want)
			}
		})
	}
}

func TestCheckPullConflicts(t *testing.T) {
	tests := []struct {
		name  string
		pages []pullPage
		want  int
	}{
		{
			name:  "no pages",
			pages: nil,
			want:  0,
		},
		{
			name: "no conflicts",
			pages: []pullPage{
				{localPath: "a.md", state: &state.SyncState{Status: "synced"}},
				{localPath: "b.md", state: nil},
			},
			want: 0,
		},
		{
			name: "with conflicts",
			pages: []pullPage{
				{localPath: "a.md", state: &state.SyncState{Status: "conflict"}},
				{localPath: "b.md", state: &state.SyncState{Status: "synced"}},
			},
			want: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := checkPullConflicts(tc.pages)
			if len(got) != tc.want {
				t.Errorf("checkPullConflicts() returned %d conflicts; want %d", len(got), tc.want)
			}
		})
	}
}

func TestFilterPullByPath(t *testing.T) {
	pages := []pullPage{
		{localPath: "work/project.md"},
		{localPath: "work/notes.md"},
		{localPath: "personal/journal.md"},
	}

	tests := []struct {
		name    string
		pattern string
		want    int
	}{
		{"match work", "work/*.md", 2},
		{"match personal", "personal/*.md", 1},
		{"match none", "archive/*.md", 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := filterPullByPath(pages, tc.pattern)
			if len(got) != tc.want {
				t.Errorf("filterPullByPath(pattern=%q) returned %d pages; want %d", tc.pattern, len(got), tc.want)
			}
		})
	}
}

// =============================================================================
// ConflictStrategy Tests
// =============================================================================

func TestConflictStrategy_ValidStrategies(t *testing.T) {
	strategies := []ConflictStrategy{
		StrategyOurs,
		StrategyTheirs,
		StrategyManual,
		StrategyNewer,
	}

	expected := []string{"ours", "theirs", "manual", "newer"}

	for i, s := range strategies {
		if string(s) != expected[i] {
			t.Errorf("ConflictStrategy %d = %q; want %q", i, s, expected[i])
		}
	}
}

// =============================================================================
// Command Structure Tests
// =============================================================================

func TestRootCommand_HasExpectedSubcommands(t *testing.T) {
	// Check that all expected subcommands are registered.
	expectedCommands := []string{
		"init",
		"push",
		"pull",
		"sync",
		"status",
		"conflicts",
		"links",
	}

	for _, cmdName := range expectedCommands {
		found := false
		for _, cmd := range rootCmd.Commands() {
			if cmd.Name() == cmdName {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("rootCmd missing expected subcommand: %s", cmdName)
		}
	}
}

func TestRootCommand_HasExpectedFlags(t *testing.T) {
	// Check persistent flags.
	configFlag := rootCmd.PersistentFlags().Lookup("config")
	if configFlag == nil {
		t.Error("rootCmd missing --config flag")
	}

	verboseFlag := rootCmd.PersistentFlags().Lookup("verbose")
	if verboseFlag == nil {
		t.Error("rootCmd missing --verbose flag")
	}
	if verboseFlag.Shorthand != "v" {
		t.Errorf("--verbose shorthand = %q; want 'v'", verboseFlag.Shorthand)
	}
}

func TestInitCommand_HasRequiredFlags(t *testing.T) {
	// Check that init command has required flags.
	vaultFlag := initCmd.Flags().Lookup("vault")
	if vaultFlag == nil {
		t.Error("initCmd missing --vault flag")
	}

	tokenFlag := initCmd.Flags().Lookup("notion-token")
	if tokenFlag == nil {
		t.Error("initCmd missing --notion-token flag")
	}

	dbFlag := initCmd.Flags().Lookup("database")
	if dbFlag == nil {
		t.Error("initCmd missing --database flag")
	}

	pageFlag := initCmd.Flags().Lookup("page")
	if pageFlag == nil {
		t.Error("initCmd missing --page flag")
	}
}

func TestPushCommand_HasExpectedFlags(t *testing.T) {
	flags := []string{"all", "path", "dry-run", "force"}
	for _, flagName := range flags {
		flag := pushCmd.Flags().Lookup(flagName)
		if flag == nil {
			t.Errorf("pushCmd missing --%s flag", flagName)
		}
	}
}

func TestPullCommand_HasExpectedFlags(t *testing.T) {
	flags := []string{"all", "path", "dry-run", "force"}
	for _, flagName := range flags {
		flag := pullCmd.Flags().Lookup(flagName)
		if flag == nil {
			t.Errorf("pullCmd missing --%s flag", flagName)
		}
	}
}

func TestSyncCommand_HasExpectedFlags(t *testing.T) {
	flags := []string{"strategy", "dry-run"}
	for _, flagName := range flags {
		flag := syncCmd.Flags().Lookup(flagName)
		if flag == nil {
			t.Errorf("syncCmd missing --%s flag", flagName)
		}
	}

	// Check default value for strategy.
	strategyFlag := syncCmd.Flags().Lookup("strategy")
	if strategyFlag.DefValue != "manual" {
		t.Errorf("syncCmd --strategy default = %q; want 'manual'", strategyFlag.DefValue)
	}
}

func TestStatusCommand_HasExpectedFlags(t *testing.T) {
	allFlag := statusCmd.Flags().Lookup("all")
	if allFlag == nil {
		t.Error("statusCmd missing --all flag")
	}
	if allFlag.Shorthand != "a" {
		t.Errorf("--all shorthand = %q; want 'a'", allFlag.Shorthand)
	}
}

func TestLinksCommand_HasExpectedFlags(t *testing.T) {
	flags := map[string]string{
		"repair":      "r",
		"dry-run":     "n",
		"suggestions": "s",
	}
	for flagName, shorthand := range flags {
		flag := linksCmd.Flags().Lookup(flagName)
		if flag == nil {
			t.Errorf("linksCmd missing --%s flag", flagName)
		} else if flag.Shorthand != shorthand {
			t.Errorf("linksCmd --%s shorthand = %q; want %q", flagName, flag.Shorthand, shorthand)
		}
	}
}

func TestConflictsCommand_HasResolveSubcommand(t *testing.T) {
	found := false
	for _, cmd := range conflictsCmd.Commands() {
		if cmd.Name() == "resolve" {
			found = true
			// Check that resolve has --keep flag.
			keepFlag := cmd.Flags().Lookup("keep")
			if keepFlag == nil {
				t.Error("resolve subcommand missing --keep flag")
			}
			break
		}
	}
	if !found {
		t.Error("conflictsCmd missing 'resolve' subcommand")
	}
}

// =============================================================================
// Error Message Tests
// =============================================================================

func TestErrNoConfig_Message(t *testing.T) {
	expected := "no configuration found - run 'obsidian-notion init' first"
	if ErrNoConfig.Error() != expected {
		t.Errorf("ErrNoConfig.Error() = %q; want %q", ErrNoConfig.Error(), expected)
	}
}

// =============================================================================
// extractTitle Tests (from pull.go)
// =============================================================================

func TestExtractTitle(t *testing.T) {
	// Note: This tests the internal function with nil/empty properties.
	// The function iterates through Properties looking for TitleProperty type.

	// Empty properties should return empty string
	got := extractTitle(nil)
	if got != "" {
		t.Errorf("extractTitle(nil) = %q; want empty string", got)
	}
}

// =============================================================================
// Watch Command Tests
// =============================================================================

func TestWatchCommand_HasExpectedSubcommands(t *testing.T) {
	expectedSubcommands := []string{"stop", "status"}

	for _, cmdName := range expectedSubcommands {
		found := false
		for _, cmd := range watchCmd.Commands() {
			if cmd.Name() == cmdName {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("watchCmd missing expected subcommand: %s", cmdName)
		}
	}
}

func TestWatchCommand_HasExpectedFlags(t *testing.T) {
	flags := []string{"debounce", "poll-interval", "daemon", "pid-file", "log-file", "strategy"}
	for _, flagName := range flags {
		flag := watchCmd.Flags().Lookup(flagName)
		if flag == nil {
			t.Errorf("watchCmd missing --%s flag", flagName)
		}
	}

	// Check strategy default value
	strategyFlag := watchCmd.Flags().Lookup("strategy")
	if strategyFlag != nil && strategyFlag.DefValue != "manual" {
		t.Errorf("watchCmd --strategy default = %q; want 'manual'", strategyFlag.DefValue)
	}
}

func TestCheckPIDFile_NonExistent(t *testing.T) {
	// Test with a file that doesn't exist
	pid, running := checkPIDFile("/nonexistent/path/to/pid/file")
	if running {
		t.Error("checkPIDFile for non-existent file should return running=false")
	}
	if pid != 0 {
		t.Errorf("checkPIDFile for non-existent file returned pid=%d; want 0", pid)
	}
}

func TestCheckPIDFile_InvalidContent(t *testing.T) {
	// Create a temp file with invalid content
	tmpFile, err := os.CreateTemp("", "pidfile-test")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	// Write invalid content (not a number)
	if _, err := tmpFile.WriteString("not-a-number"); err != nil {
		t.Fatalf("Failed to write to temp file: %v", err)
	}
	tmpFile.Close()

	pid, running := checkPIDFile(tmpFile.Name())
	if running {
		t.Error("checkPIDFile with invalid content should return running=false")
	}
	if pid != 0 {
		t.Errorf("checkPIDFile with invalid content returned pid=%d; want 0", pid)
	}
}

func TestCheckPIDFile_CurrentProcess(t *testing.T) {
	// Create a temp file with our current PID
	tmpFile, err := os.CreateTemp("", "pidfile-test")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	// Write current process PID
	currentPID := os.Getpid()
	if _, err := tmpFile.WriteString(fmt.Sprintf("%d", currentPID)); err != nil {
		t.Fatalf("Failed to write to temp file: %v", err)
	}
	tmpFile.Close()

	pid, running := checkPIDFile(tmpFile.Name())
	if !running {
		t.Error("checkPIDFile with current process PID should return running=true")
	}
	if pid != currentPID {
		t.Errorf("checkPIDFile returned pid=%d; want %d", pid, currentPID)
	}
}

func TestCheckPIDFile_NonRunningProcess(t *testing.T) {
	// Create a temp file with a very high PID that's unlikely to be running
	tmpFile, err := os.CreateTemp("", "pidfile-test")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	// Write a PID that's almost certainly not running (max PID on most systems)
	// Note: This might occasionally fail if that PID happens to exist
	fakePID := 4194304 // Common max PID on Linux
	if _, err := tmpFile.WriteString(fmt.Sprintf("%d", fakePID)); err != nil {
		t.Fatalf("Failed to write to temp file: %v", err)
	}
	tmpFile.Close()

	_, running := checkPIDFile(tmpFile.Name())
	// We can't guarantee this will be false on all systems, but it usually will be
	// Just verify the function doesn't panic
	_ = running
}

// =============================================================================
// Watcher shouldIgnore Tests
// =============================================================================

func TestWatcher_ShouldIgnore(t *testing.T) {
	w := &watcher{
		cfg: &config.Config{
			Sync: config.SyncConfig{
				Ignore: []string{"*.tmp", "drafts/*", "templates"},
			},
		},
	}

	tests := []struct {
		path   string
		want   bool
	}{
		{"notes.md", false},
		{"work/project.md", false},
		{"file.tmp", true},
		{"notes.tmp", true},
		{"drafts/wip.md", true},
		{"templates", true},
		{"other/templates", true}, // Base name also matches pattern
	}

	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			got := w.shouldIgnore(tc.path)
			if got != tc.want {
				t.Errorf("shouldIgnore(%q) = %v; want %v", tc.path, got, tc.want)
			}
		})
	}
}

func TestWatcher_ShouldIgnore_EmptyPatterns(t *testing.T) {
	w := &watcher{
		cfg: &config.Config{
			Sync: config.SyncConfig{
				Ignore: nil,
			},
		},
	}

	// With no ignore patterns, nothing should be ignored
	if w.shouldIgnore("any/path.md") {
		t.Error("shouldIgnore should return false when no patterns are configured")
	}
}

// =============================================================================
// pullChangeType Tests
// =============================================================================

func TestPullChangeType_Values(t *testing.T) {
	// Verify the enum values are as expected
	if pullChangeNew != 0 {
		t.Errorf("pullChangeNew = %d; want 0", pullChangeNew)
	}
	if pullChangeModified != 1 {
		t.Errorf("pullChangeModified = %d; want 1", pullChangeModified)
	}
	if pullChangeDeleted != 2 {
		t.Errorf("pullChangeDeleted = %d; want 2", pullChangeDeleted)
	}
}

// =============================================================================
// Additional Helper Function Tests
// =============================================================================

func TestFilterByPath_NestedPatterns(t *testing.T) {
	files := []pushFile{
		{path: "a/b/c/d.md"},
		{path: "x/y.md"},
		{path: "single.md"},
	}

	// Test patterns that should not match
	filtered := filterByPath(files, "z/*.md")
	if len(filtered) != 0 {
		t.Errorf("filterByPath with non-matching pattern returned %d files; want 0", len(filtered))
	}

	// Empty pattern should match nothing (filepath.Match behavior)
	filtered = filterByPath(files, "")
	if len(filtered) != 0 {
		t.Errorf("filterByPath with empty pattern returned %d files; want 0", len(filtered))
	}
}

func TestFilterPullByPath_NestedPatterns(t *testing.T) {
	pages := []pullPage{
		{localPath: "a/b/c/d.md"},
		{localPath: "x/y.md"},
	}

	// Test exact pattern matching
	filtered := filterPullByPath(pages, "x/y.md")
	if len(filtered) != 1 {
		t.Errorf("filterPullByPath with exact pattern returned %d pages; want 1", len(filtered))
	}
}

// =============================================================================
// Command Use/Short/Long Description Tests
// =============================================================================

func TestCommandDescriptions(t *testing.T) {
	commands := []struct {
		cmd   *cobra.Command
		name  string
	}{
		{initCmd, "init"},
		{pushCmd, "push"},
		{pullCmd, "pull"},
		{syncCmd, "sync"},
		{statusCmd, "status"},
		{conflictsCmd, "conflicts"},
		{linksCmd, "links"},
		{watchCmd, "watch"},
	}

	for _, tc := range commands {
		t.Run(tc.name, func(t *testing.T) {
			if tc.cmd.Use == "" {
				t.Errorf("%s command has empty Use", tc.name)
			}
			if tc.cmd.Short == "" {
				t.Errorf("%s command has empty Short description", tc.name)
			}
			if tc.cmd.Long == "" {
				t.Errorf("%s command has empty Long description", tc.name)
			}
		})
	}
}

func TestResolveCommand_Description(t *testing.T) {
	if resolveCmd.Use == "" {
		t.Error("resolveCmd has empty Use")
	}
	if resolveCmd.Short == "" {
		t.Error("resolveCmd has empty Short description")
	}
	// Check that it requires exactly 1 argument
	err := resolveCmd.Args(resolveCmd, []string{})
	if err == nil {
		t.Error("resolveCmd should require exactly 1 argument, but accepted 0")
	}
	err = resolveCmd.Args(resolveCmd, []string{"path/to/file.md"})
	if err != nil {
		t.Errorf("resolveCmd should accept exactly 1 argument: %v", err)
	}
	err = resolveCmd.Args(resolveCmd, []string{"path1.md", "path2.md"})
	if err == nil {
		t.Error("resolveCmd should require exactly 1 argument, but accepted 2")
	}
}

// =============================================================================
// syncResult Tests
// =============================================================================

func TestSyncResult_Fields(t *testing.T) {
	result := syncResult{
		Pushed:        5,
		Pulled:        3,
		Conflicts:     1,
		ConflictPaths: []string{"file1.md", "file2.md"},
		Failed:        2,
	}

	if result.Pushed != 5 {
		t.Errorf("syncResult.Pushed = %d; want 5", result.Pushed)
	}
	if result.Pulled != 3 {
		t.Errorf("syncResult.Pulled = %d; want 3", result.Pulled)
	}
	if result.Conflicts != 1 {
		t.Errorf("syncResult.Conflicts = %d; want 1", result.Conflicts)
	}
	if len(result.ConflictPaths) != 2 {
		t.Errorf("len(syncResult.ConflictPaths) = %d; want 2", len(result.ConflictPaths))
	}
	if result.Failed != 2 {
		t.Errorf("syncResult.Failed = %d; want 2", result.Failed)
	}
}

// =============================================================================
// Integration Test with Temp Directory
// =============================================================================

func TestExpandAndValidateVaultPath_WithSubdirectory(t *testing.T) {
	// Create a temp directory with nested structure
	tmpDir, err := os.MkdirTemp("", "vault-integration-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create subdirectory
	subDir := filepath.Join(tmpDir, "notes", "work")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("Failed to create subdirectory: %v", err)
	}

	// Test with the nested subdirectory
	result, err := expandAndValidateVaultPath(subDir)
	if err != nil {
		t.Errorf("expandAndValidateVaultPath(%q) unexpected error: %v", subDir, err)
	}
	if result != subDir {
		t.Errorf("expandAndValidateVaultPath(%q) = %q; want %q", subDir, result, subDir)
	}
}

// =============================================================================
// pushFile and pullPage Tests
// =============================================================================

func TestPushFile_Fields(t *testing.T) {
	pf := pushFile{
		path:       "notes/test.md",
		oldPath:    "notes/old.md",
		state:      &state.SyncState{Status: "synced"},
		changeType: state.ChangeRenamed,
	}

	if pf.path != "notes/test.md" {
		t.Errorf("pushFile.path = %q; want 'notes/test.md'", pf.path)
	}
	if pf.oldPath != "notes/old.md" {
		t.Errorf("pushFile.oldPath = %q; want 'notes/old.md'", pf.oldPath)
	}
	if pf.changeType != state.ChangeRenamed {
		t.Errorf("pushFile.changeType = %v; want ChangeRenamed", pf.changeType)
	}
}

func TestPullPage_Fields(t *testing.T) {
	testTime := time.Now()
	pp := pullPage{
		notionPageID: "abc123",
		localPath:    "notes/test.md",
		state:        &state.SyncState{Status: "synced"},
		notionMtime:  testTime,
		changeType:   pullChangeModified,
	}

	if pp.notionPageID != "abc123" {
		t.Errorf("pullPage.notionPageID = %q; want 'abc123'", pp.notionPageID)
	}
	if pp.localPath != "notes/test.md" {
		t.Errorf("pullPage.localPath = %q; want 'notes/test.md'", pp.localPath)
	}
	if pp.changeType != pullChangeModified {
		t.Errorf("pullPage.changeType = %v; want pullChangeModified", pp.changeType)
	}
	if pp.notionMtime != testTime {
		t.Errorf("pullPage.notionMtime mismatch")
	}
}

// =============================================================================
// extractTitle Tests with Notion Properties
// =============================================================================

func TestExtractTitle_WithTitleProperty(t *testing.T) {
	// Create a properties map with a title property
	props := notionapi.Properties{
		"Name": &notionapi.TitleProperty{
			ID:   "title",
			Type: notionapi.PropertyTypeTitle,
			Title: []notionapi.RichText{
				{
					Type:      notionapi.ObjectTypeText,
					PlainText: "My Note Title",
				},
			},
		},
	}

	got := extractTitle(props)
	if got != "My Note Title" {
		t.Errorf("extractTitle with title property = %q; want 'My Note Title'", got)
	}
}

func TestExtractTitle_EmptyTitle(t *testing.T) {
	// Create a properties map with an empty title property
	props := notionapi.Properties{
		"Name": &notionapi.TitleProperty{
			ID:    "title",
			Type:  notionapi.PropertyTypeTitle,
			Title: []notionapi.RichText{},
		},
	}

	got := extractTitle(props)
	if got != "" {
		t.Errorf("extractTitle with empty title = %q; want ''", got)
	}
}

func TestExtractTitle_NoTitleProperty(t *testing.T) {
	// Create a properties map without a title property
	props := notionapi.Properties{
		"Status": &notionapi.SelectProperty{
			ID:   "status",
			Type: notionapi.PropertyTypeSelect,
		},
	}

	got := extractTitle(props)
	if got != "" {
		t.Errorf("extractTitle without title property = %q; want ''", got)
	}
}

// =============================================================================
// extractDatabaseTitle Tests
// =============================================================================

func TestExtractDatabaseTitle_NilDatabase(t *testing.T) {
	got := extractDatabaseTitle(nil)
	if got != "" {
		t.Errorf("extractDatabaseTitle(nil) = %q; want ''", got)
	}
}

func TestExtractDatabaseTitle_EmptyTitle(t *testing.T) {
	db := &notionapi.Database{
		Title: []notionapi.RichText{},
	}
	got := extractDatabaseTitle(db)
	if got != "" {
		t.Errorf("extractDatabaseTitle with empty title = %q; want ''", got)
	}
}

func TestExtractDatabaseTitle_SinglePart(t *testing.T) {
	db := &notionapi.Database{
		Title: []notionapi.RichText{
			{PlainText: "Notes Database"},
		},
	}
	got := extractDatabaseTitle(db)
	if got != "Notes Database" {
		t.Errorf("extractDatabaseTitle = %q; want 'Notes Database'", got)
	}
}

func TestExtractDatabaseTitle_MultipleParts(t *testing.T) {
	db := &notionapi.Database{
		Title: []notionapi.RichText{
			{PlainText: "Work "},
			{PlainText: "Notes "},
			{PlainText: "2024"},
		},
	}
	got := extractDatabaseTitle(db)
	if got != "Work Notes 2024" {
		t.Errorf("extractDatabaseTitle = %q; want 'Work Notes 2024'", got)
	}
}

// =============================================================================
// Additional pushFile Tests
// =============================================================================

func TestPushFile_AllFields(t *testing.T) {
	testTime := time.Now()
	pf := pushFile{
		path:       "notes/test.md",
		oldPath:    "notes/old.md",
		state:      &state.SyncState{Status: "synced", NotionPageID: "page123"},
		mtime:      testTime,
		changeType: state.ChangeModified,
	}

	if pf.mtime != testTime {
		t.Error("pushFile.mtime mismatch")
	}
	if pf.state.NotionPageID != "page123" {
		t.Errorf("pushFile.state.NotionPageID = %q; want 'page123'", pf.state.NotionPageID)
	}
}

// =============================================================================
// watcher struct Tests
// =============================================================================

func TestWatcher_PendingChangesMap(t *testing.T) {
	w := &watcher{
		cfg: &config.Config{
			Vault: "/test/vault",
		},
		pendingChanges: make(map[string]time.Time),
		debounce:       5 * time.Second,
	}

	// Simulate adding pending changes
	w.pendingChanges["test.md"] = time.Now()
	w.pendingChanges["other.md"] = time.Now().Add(-10 * time.Second)

	if len(w.pendingChanges) != 2 {
		t.Errorf("watcher.pendingChanges length = %d; want 2", len(w.pendingChanges))
	}
}

// =============================================================================
// pushContext and pullContext Tests
// =============================================================================

func TestPushContext_Fields(t *testing.T) {
	// Test that pushContext can be created with expected fields
	pc := &pushContext{
		cfg: &config.Config{
			Vault: "/test/vault",
		},
	}

	if pc.cfg.Vault != "/test/vault" {
		t.Errorf("pushContext.cfg.Vault = %q; want '/test/vault'", pc.cfg.Vault)
	}
}

func TestPullContext_Fields(t *testing.T) {
	// Test that pullContext can be created with expected fields
	pc := &pullContext{
		cfg: &config.Config{
			Vault: "/test/vault",
		},
	}

	if pc.cfg.Vault != "/test/vault" {
		t.Errorf("pullContext.cfg.Vault = %q; want '/test/vault'", pc.cfg.Vault)
	}
}

func TestPushResult_Fields(t *testing.T) {
	pr := pushResult{
		pageID:       "page123",
		isNew:        true,
		hasWikiLinks: true,
	}

	if pr.pageID != "page123" {
		t.Errorf("pushResult.pageID = %q; want 'page123'", pr.pageID)
	}
	if !pr.isNew {
		t.Error("pushResult.isNew should be true")
	}
	if !pr.hasWikiLinks {
		t.Error("pushResult.hasWikiLinks should be true")
	}
}

func TestPullResult_Fields(t *testing.T) {
	pr := pullResult{
		isNew: true,
	}

	if !pr.isNew {
		t.Error("pullResult.isNew should be true")
	}
}

// =============================================================================
// syncPushContext and syncPullContext Tests
// =============================================================================

func TestSyncPushContext_Fields(t *testing.T) {
	spc := &syncPushContext{
		cfg: &config.Config{
			Vault: "/test/vault",
		},
	}

	if spc.cfg.Vault != "/test/vault" {
		t.Errorf("syncPushContext.cfg.Vault = %q; want '/test/vault'", spc.cfg.Vault)
	}
}

func TestSyncPullContext_Fields(t *testing.T) {
	spc := &syncPullContext{
		cfg: &config.Config{
			Vault: "/test/vault",
		},
	}

	if spc.cfg.Vault != "/test/vault" {
		t.Errorf("syncPullContext.cfg.Vault = %q; want '/test/vault'", spc.cfg.Vault)
	}
}

// =============================================================================
// Root Command PersistentPreRunE Tests
// =============================================================================

func TestRootCommand_VersionTemplate(t *testing.T) {
	// Test that version template is set
	template := rootCmd.VersionTemplate()
	if template == "" {
		t.Error("rootCmd should have a version template set")
	}
}

func TestRootCommand_UsageDescription(t *testing.T) {
	if rootCmd.Use != "obsidian-notion" {
		t.Errorf("rootCmd.Use = %q; want 'obsidian-notion'", rootCmd.Use)
	}
	if rootCmd.Short == "" {
		t.Error("rootCmd should have a Short description")
	}
	if rootCmd.Long == "" {
		t.Error("rootCmd should have a Long description")
	}
}
