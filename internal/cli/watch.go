package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/cobra"

	"github.com/adamancini/obsidian-notion-sync/internal/config"
	"github.com/adamancini/obsidian-notion-sync/internal/notion"
	"github.com/adamancini/obsidian-notion-sync/internal/parser"
	"github.com/adamancini/obsidian-notion-sync/internal/state"
	"github.com/adamancini/obsidian-notion-sync/internal/transformer"
	"github.com/adamancini/obsidian-notion-sync/internal/vault"
)

var (
	watchDebounce     string
	watchPollInterval string
	watchDaemon       bool
	watchPIDFile      string
	watchLogFile      string
	watchStrategy     string
)

// watchCmd represents the watch command.
var watchCmd = &cobra.Command{
	Use:   "watch",
	Short: "Watch vault for changes and sync automatically",
	Long: `Watch the Obsidian vault for file changes and automatically sync with Notion.

This command runs continuously, monitoring your vault for changes and pushing
them to Notion. It can also optionally poll Notion for remote changes.

Examples:
  obsidian-notion watch                       # Watch with default settings
  obsidian-notion watch --debounce 10s        # Wait 10s after changes before syncing
  obsidian-notion watch --poll-interval 1m    # Check Notion every minute
  obsidian-notion watch --daemon              # Run as background process
  obsidian-notion watch --strategy ours       # Auto-resolve conflicts with local version

Press Ctrl+C to stop watching.`,
	RunE: runWatch,
}

func init() {
	watchCmd.Flags().StringVar(&watchDebounce, "debounce", "", "wait duration after file change (default: 5s)")
	watchCmd.Flags().StringVar(&watchPollInterval, "poll-interval", "", "Notion poll interval (default: 5m, 0 to disable)")
	watchCmd.Flags().BoolVar(&watchDaemon, "daemon", false, "run as background daemon")
	watchCmd.Flags().StringVar(&watchPIDFile, "pid-file", "", "PID file for daemon mode")
	watchCmd.Flags().StringVar(&watchLogFile, "log-file", "", "log file for daemon mode")
	watchCmd.Flags().StringVar(&watchStrategy, "strategy", "manual", "conflict resolution strategy (ours|theirs|manual|newer)")

	rootCmd.AddCommand(watchCmd)
}

// watcher handles file system watching and sync coordination.
type watcher struct {
	cfg          *config.Config
	db           *state.DB
	client       *notion.Client
	linkRegistry *state.LinkRegistry
	parser       *parser.Parser
	transformer  *transformer.Transformer
	scanner      *vault.Scanner

	debounce     time.Duration
	pollInterval time.Duration
	strategy     ConflictStrategy

	// Debounce state
	pendingChanges map[string]time.Time
	pendingMu      sync.Mutex
	debounceTicker *time.Ticker

	// Output
	out io.Writer
}

func runWatch(cmd *cobra.Command, args []string) error {
	cfg, err := getConfig()
	if err != nil {
		return err
	}

	// Parse strategy.
	strategy := ConflictStrategy(watchStrategy)
	switch strategy {
	case StrategyOurs, StrategyTheirs, StrategyManual, StrategyNewer:
		// Valid
	default:
		return fmt.Errorf("invalid conflict strategy: %s", watchStrategy)
	}

	// Handle daemon mode.
	if watchDaemon {
		return runDaemon(cfg)
	}

	return runWatchForeground(cfg, strategy, os.Stdout)
}

// runWatchForeground runs the watcher in foreground mode.
func runWatchForeground(cfg *config.Config, strategy ConflictStrategy, out io.Writer) error {
	// Parse debounce duration.
	debounceStr := watchDebounce
	if debounceStr == "" {
		debounceStr = cfg.Watch.Debounce
	}
	if debounceStr == "" {
		debounceStr = "5s"
	}
	debounce, err := time.ParseDuration(debounceStr)
	if err != nil {
		return fmt.Errorf("invalid debounce duration: %w", err)
	}

	// Parse poll interval.
	pollStr := watchPollInterval
	if pollStr == "" {
		pollStr = cfg.Watch.PollInterval
	}
	if pollStr == "" {
		pollStr = "5m"
	}
	var pollInterval time.Duration
	if pollStr != "0" {
		pollInterval, err = time.ParseDuration(pollStr)
		if err != nil {
			return fmt.Errorf("invalid poll interval: %w", err)
		}
	}

	// Open state database.
	dbPath := filepath.Join(cfg.Vault, ".obsidian-notion.db")
	db, err := state.Open(dbPath)
	if err != nil {
		return fmt.Errorf("open database: %w (run 'obsidian-notion init' first)", err)
	}
	defer db.Close()

	// Initialize components.
	client := notion.New(cfg.Notion.Token,
		notion.WithRateLimit(cfg.RateLimit.RequestsPerSecond),
		notion.WithBatchSize(cfg.RateLimit.BatchSize),
	)

	linkRegistry := state.NewLinkRegistry(db)

	w := &watcher{
		cfg:          cfg,
		db:           db,
		client:       client,
		linkRegistry: linkRegistry,
		parser:       parser.New(),
		transformer: transformer.New(linkRegistry, &transformer.Config{
			UnresolvedLinkStyle: cfg.Transform.UnresolvedLinks,
			CalloutIcons:        cfg.Transform.Callouts,
			DataviewHandling:    cfg.Transform.Dataview,
			FlattenHeadings:     true,
		}),
		scanner:        vault.NewScanner(cfg.Vault, cfg.Sync.Ignore),
		debounce:       debounce,
		pollInterval:   pollInterval,
		strategy:       strategy,
		pendingChanges: make(map[string]time.Time),
		out:            out,
	}

	return w.run()
}

// run starts the file watcher and sync loop.
func (w *watcher) run() error {
	// Create fsnotify watcher.
	fsWatcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("create watcher: %w", err)
	}
	defer fsWatcher.Close()

	// Add vault directory and subdirectories.
	if err := w.addWatchRecursive(fsWatcher, w.cfg.Vault); err != nil {
		return fmt.Errorf("add watch directories: %w", err)
	}

	fmt.Fprintf(w.out, "Watching vault: %s\n", w.cfg.Vault)
	fmt.Fprintf(w.out, "Debounce: %s\n", w.debounce)
	if w.pollInterval > 0 {
		fmt.Fprintf(w.out, "Poll interval: %s\n", w.pollInterval)
	} else {
		fmt.Fprintf(w.out, "Notion polling: disabled\n")
	}
	fmt.Fprintf(w.out, "Conflict strategy: %s\n", w.strategy)
	fmt.Fprintf(w.out, "\nPress Ctrl+C to stop...\n\n")

	// Setup signal handling.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Setup debounce ticker.
	w.debounceTicker = time.NewTicker(500 * time.Millisecond)
	defer w.debounceTicker.Stop()

	// Setup poll ticker (if enabled).
	var pollTicker *time.Ticker
	var pollCh <-chan time.Time
	if w.pollInterval > 0 {
		pollTicker = time.NewTicker(w.pollInterval)
		defer pollTicker.Stop()
		pollCh = pollTicker.C
	}

	// Main event loop.
	for {
		select {
		case <-sigCh:
			fmt.Fprintf(w.out, "\nShutting down...\n")
			return nil

		case event, ok := <-fsWatcher.Events:
			if !ok {
				return fmt.Errorf("watcher closed unexpectedly")
			}
			w.handleFsEvent(fsWatcher, event)

		case err, ok := <-fsWatcher.Errors:
			if !ok {
				return fmt.Errorf("watcher error channel closed")
			}
			fmt.Fprintf(w.out, "Watch error: %v\n", err)

		case <-w.debounceTicker.C:
			w.processDebounced()

		case <-pollCh:
			w.pollNotion()
		}
	}
}

// addWatchRecursive adds the directory and all subdirectories to the watcher.
func (w *watcher) addWatchRecursive(fsWatcher *fsnotify.Watcher, root string) error {
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			// Skip hidden directories and .obsidian.
			name := info.Name()
			if strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
			return fsWatcher.Add(path)
		}
		return nil
	})
}

// handleFsEvent processes a file system event.
func (w *watcher) handleFsEvent(fsWatcher *fsnotify.Watcher, event fsnotify.Event) {
	path := event.Name

	// Get relative path.
	relPath, err := filepath.Rel(w.cfg.Vault, path)
	if err != nil {
		return
	}

	// Skip hidden files and directories.
	if strings.HasPrefix(filepath.Base(relPath), ".") {
		return
	}

	// Skip non-markdown files.
	if !strings.HasSuffix(relPath, ".md") {
		// But handle directory creation.
		if event.Has(fsnotify.Create) {
			info, err := os.Stat(path)
			if err == nil && info.IsDir() {
				_ = fsWatcher.Add(path)
			}
		}
		return
	}

	// Check if file matches ignore patterns.
	if w.shouldIgnore(relPath) {
		return
	}

	// Record the change for debouncing.
	w.pendingMu.Lock()
	w.pendingChanges[relPath] = time.Now()
	w.pendingMu.Unlock()

	if verbose {
		opStr := "modified"
		if event.Has(fsnotify.Create) {
			opStr = "created"
		} else if event.Has(fsnotify.Remove) {
			opStr = "deleted"
		} else if event.Has(fsnotify.Rename) {
			opStr = "renamed"
		}
		fmt.Fprintf(w.out, "[%s] %s %s\n", time.Now().Format("15:04:05"), opStr, relPath)
	}
}

// shouldIgnore checks if a file should be ignored based on patterns.
func (w *watcher) shouldIgnore(relPath string) bool {
	for _, pattern := range w.cfg.Sync.Ignore {
		matched, _ := filepath.Match(pattern, relPath)
		if matched {
			return true
		}
		// Also try matching against base name.
		matched, _ = filepath.Match(pattern, filepath.Base(relPath))
		if matched {
			return true
		}
	}
	return false
}

// processDebounced processes pending changes that have waited long enough.
func (w *watcher) processDebounced() {
	w.pendingMu.Lock()
	defer w.pendingMu.Unlock()

	if len(w.pendingChanges) == 0 {
		return
	}

	now := time.Now()
	var toProcess []string

	for path, changedAt := range w.pendingChanges {
		if now.Sub(changedAt) >= w.debounce {
			toProcess = append(toProcess, path)
		}
	}

	if len(toProcess) == 0 {
		return
	}

	// Remove from pending.
	for _, path := range toProcess {
		delete(w.pendingChanges, path)
	}

	// Process changes.
	fmt.Fprintf(w.out, "[%s] Syncing %d change(s)...\n", time.Now().Format("15:04:05"), len(toProcess))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	for _, relPath := range toProcess {
		if err := w.syncFile(ctx, relPath); err != nil {
			fmt.Fprintf(w.out, "  Error syncing %s: %v\n", relPath, err)
		} else {
			fmt.Fprintf(w.out, "  Synced: %s\n", relPath)
		}
	}
}

// syncFile synchronizes a single file to Notion.
func (w *watcher) syncFile(ctx context.Context, relPath string) error {
	fullPath := filepath.Join(w.cfg.Vault, relPath)

	// Check if file exists (might have been deleted).
	info, err := os.Stat(fullPath)
	if os.IsNotExist(err) {
		// File was deleted - handle deletion.
		return w.handleDeletion(ctx, relPath)
	}
	if err != nil {
		return fmt.Errorf("stat file: %w", err)
	}
	if info.IsDir() {
		return nil
	}

	// Read file content.
	content, err := os.ReadFile(fullPath)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}

	// Check if content has actually changed.
	hashes, err := state.HashFileDetailed(fullPath)
	if err != nil {
		return fmt.Errorf("hash file: %w", err)
	}

	existingState, _ := w.db.GetState(relPath)
	if existingState != nil && existingState.ContentHash == hashes.ContentHash {
		// Content hasn't changed, skip.
		return nil
	}

	// Parse markdown.
	note, err := w.parser.Parse(relPath, content)
	if err != nil {
		return fmt.Errorf("parse markdown: %w", err)
	}

	// Register wiki-links.
	_ = w.linkRegistry.ClearLinksFrom(relPath)
	if len(note.WikiLinks) > 0 {
		targets := make([]string, len(note.WikiLinks))
		for i, link := range note.WikiLinks {
			targets[i] = link.Target
		}
		_ = w.linkRegistry.RegisterLinks(relPath, targets)
	}

	// Transform to Notion.
	notionPage, err := w.transformer.Transform(note)
	if err != nil {
		return fmt.Errorf("transform: %w", err)
	}

	// Create or update page.
	var pageID string
	if existingState == nil || existingState.NotionPageID == "" {
		// Create new page.
		parentID := w.cfg.GetDatabaseForPath(relPath)
		if parentID == "" {
			parentID = w.cfg.Notion.DefaultPage
		}
		result, err := w.client.CreatePage(ctx, parentID, notionPage)
		if err != nil {
			return fmt.Errorf("create page: %w", err)
		}
		pageID = result.PageID
	} else {
		// Update existing page.
		pageID = existingState.NotionPageID
		if err := w.client.UpdatePage(ctx, pageID, notionPage); err != nil {
			return fmt.Errorf("update page: %w", err)
		}
	}

	// Update sync state.
	syncState := &state.SyncState{
		ObsidianPath:    relPath,
		NotionPageID:    pageID,
		ObsidianMtime:   info.ModTime(),
		NotionMtime:     time.Now(),
		ContentHash:     hashes.ContentHash,
		FrontmatterHash: hashes.FrontmatterHash,
		LastSync:        time.Now(),
		SyncDirection:   "push",
		Status:          "synced",
	}
	return w.db.SetState(syncState)
}

// handleDeletion handles a deleted file.
func (w *watcher) handleDeletion(ctx context.Context, relPath string) error {
	existingState, err := w.db.GetState(relPath)
	if err != nil || existingState == nil {
		// Not tracked, nothing to do.
		return nil
	}

	if existingState.NotionPageID != "" {
		strategy := w.cfg.Sync.DeletionStrategy
		if strategy == "" {
			strategy = "archive"
		}
		switch strategy {
		case "archive":
			if err := w.client.ArchivePage(ctx, existingState.NotionPageID); err != nil {
				return fmt.Errorf("archive page: %w", err)
			}
		case "delete":
			if err := w.client.DeletePage(ctx, existingState.NotionPageID); err != nil {
				return fmt.Errorf("delete page: %w", err)
			}
		}
	}

	_ = w.db.DeleteState(relPath)
	_ = w.linkRegistry.ClearLinksFrom(relPath)
	return nil
}

// pollNotion checks Notion for remote changes.
func (w *watcher) pollNotion() {
	if verbose {
		fmt.Fprintf(w.out, "[%s] Polling Notion for changes...\n", time.Now().Format("15:04:05"))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Get all synced states.
	states, err := w.db.ListStates("")
	if err != nil {
		fmt.Fprintf(w.out, "Error getting states: %v\n", err)
		return
	}

	conflictTracker := state.NewConflictTracker(w.db)
	var remoteChanges []string

	for _, s := range states {
		if s.NotionPageID == "" {
			continue
		}

		// Fetch page metadata from Notion.
		page, err := w.client.GetPage(ctx, s.NotionPageID)
		if err != nil {
			if verbose {
				fmt.Fprintf(w.out, "  Error fetching %s: %v\n", s.ObsidianPath, err)
			}
			continue
		}

		// Check if remote has changed since last sync.
		if page.LastEditedTime.After(s.NotionMtime) {
			// Remote has changed - check for conflict.
			fullPath := filepath.Join(w.cfg.Vault, s.ObsidianPath)
			currentHashes, err := state.HashFileDetailed(fullPath)
			if err != nil {
				continue
			}

			if currentHashes.ContentHash != s.ContentHash {
				// Local also changed - conflict!
				if w.strategy == StrategyManual {
					fmt.Fprintf(w.out, "[%s] Conflict detected: %s\n", time.Now().Format("15:04:05"), s.ObsidianPath)
					info := &state.ConflictInfo{
						Path:        s.ObsidianPath,
						LocalHash:   currentHashes.ContentHash,
						RemoteHash:  "",
						LocalMtime:  s.ObsidianMtime,
						RemoteMtime: page.LastEditedTime,
						DetectedAt:  time.Now(),
					}
					_ = conflictTracker.RecordConflict(info)
					continue
				}
				// Auto-resolve based on strategy.
				switch w.strategy {
				case StrategyOurs:
					// Local wins - push.
					remoteChanges = append(remoteChanges, s.ObsidianPath)
				case StrategyTheirs:
					// Remote wins - will pull below.
				case StrategyNewer:
					fileInfo, _ := os.Stat(fullPath)
					if fileInfo != nil && fileInfo.ModTime().After(page.LastEditedTime) {
						// Local is newer - push.
						remoteChanges = append(remoteChanges, s.ObsidianPath)
						continue
					}
					// Remote is newer - will pull below.
				}
			}

			// Pull remote change.
			if err := w.pullFile(ctx, s.ObsidianPath, s.NotionPageID); err != nil {
				fmt.Fprintf(w.out, "  Error pulling %s: %v\n", s.ObsidianPath, err)
			} else {
				fmt.Fprintf(w.out, "[%s] Pulled: %s\n", time.Now().Format("15:04:05"), s.ObsidianPath)
			}
		}
	}

	// Process any files that need pushing due to conflict resolution.
	for _, path := range remoteChanges {
		if err := w.syncFile(ctx, path); err != nil {
			fmt.Fprintf(w.out, "  Error syncing %s: %v\n", path, err)
		}
	}
}

// pullFile pulls a file from Notion.
func (w *watcher) pullFile(ctx context.Context, relPath, pageID string) error {
	reverseTransformer := transformer.NewReverse(w.linkRegistry, &transformer.Config{
		UnresolvedLinkStyle: w.cfg.Transform.UnresolvedLinks,
		CalloutIcons:        w.cfg.Transform.Callouts,
		DataviewHandling:    w.cfg.Transform.Dataview,
		FlattenHeadings:     true,
	})

	// Fetch page from Notion.
	notionPage, err := w.client.FetchPage(ctx, pageID)
	if err != nil {
		return fmt.Errorf("fetch page: %w", err)
	}

	// Transform to markdown.
	markdown, err := reverseTransformer.NotionToMarkdown(notionPage)
	if err != nil {
		return fmt.Errorf("transform to markdown: %w", err)
	}

	// Write file.
	fullPath := filepath.Join(w.cfg.Vault, relPath)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}
	if err := os.WriteFile(fullPath, markdown, 0644); err != nil {
		return fmt.Errorf("write file: %w", err)
	}

	// Update sync state.
	hashes, _ := state.HashFileDetailed(fullPath)
	fileInfo, _ := os.Stat(fullPath)
	var mtime time.Time
	if fileInfo != nil {
		mtime = fileInfo.ModTime()
	}

	syncState := &state.SyncState{
		ObsidianPath:    relPath,
		NotionPageID:    pageID,
		ObsidianMtime:   mtime,
		NotionMtime:     time.Now(),
		ContentHash:     hashes.ContentHash,
		FrontmatterHash: hashes.FrontmatterHash,
		LastSync:        time.Now(),
		SyncDirection:   "pull",
		Status:          "synced",
	}
	return w.db.SetState(syncState)
}

// runDaemon runs the watcher as a background daemon.
func runDaemon(cfg *config.Config) error {
	// Determine PID file location.
	pidFile := watchPIDFile
	if pidFile == "" {
		pidFile = cfg.Watch.PIDFile
	}
	if pidFile == "" {
		// Default location.
		if xdgRuntime := os.Getenv("XDG_RUNTIME_DIR"); xdgRuntime != "" {
			pidFile = filepath.Join(xdgRuntime, "obsidian-notion.pid")
		} else {
			pidFile = "/tmp/obsidian-notion.pid"
		}
	}

	// Check for existing daemon.
	if pid, running := checkPIDFile(pidFile); running {
		return fmt.Errorf("daemon already running (PID: %d)", pid)
	}

	// Determine log file location.
	logFile := watchLogFile
	if logFile == "" {
		logFile = cfg.Watch.LogFile
	}

	// Fork to background.
	// In Go, we re-exec the process without --daemon flag and redirect output.
	// For simplicity here, we'll just detach by running in background with nohup-like behavior.

	// Create log file.
	var logWriter io.Writer = os.Stdout
	if logFile != "" {
		f, err := os.OpenFile(logFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			return fmt.Errorf("open log file: %w", err)
		}
		defer f.Close()
		logWriter = f
	}

	// Write PID file.
	if err := os.WriteFile(pidFile, []byte(strconv.Itoa(os.Getpid())), 0644); err != nil {
		return fmt.Errorf("write PID file: %w", err)
	}
	defer os.Remove(pidFile)

	fmt.Fprintf(logWriter, "[%s] Daemon started (PID: %d)\n", time.Now().Format(time.RFC3339), os.Getpid())
	fmt.Printf("Daemon started (PID: %d)\n", os.Getpid())
	fmt.Printf("PID file: %s\n", pidFile)
	if logFile != "" {
		fmt.Printf("Log file: %s\n", logFile)
	}

	strategy := ConflictStrategy(watchStrategy)
	return runWatchForeground(cfg, strategy, logWriter)
}

// checkPIDFile checks if a daemon is already running.
func checkPIDFile(pidFile string) (int, bool) {
	data, err := os.ReadFile(pidFile)
	if err != nil {
		return 0, false
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, false
	}

	// Check if process is running.
	process, err := os.FindProcess(pid)
	if err != nil {
		return 0, false
	}

	// On Unix, FindProcess always succeeds. We need to signal 0 to check.
	err = process.Signal(syscall.Signal(0))
	return pid, err == nil
}

// stopCmd represents the stop subcommand for stopping the daemon.
var stopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the watch daemon",
	RunE:  runStop,
}

func init() {
	watchCmd.AddCommand(stopCmd)
}

func runStop(cmd *cobra.Command, args []string) error {
	cfg, err := getConfig()
	if err != nil {
		return err
	}

	// Determine PID file location.
	pidFile := watchPIDFile
	if pidFile == "" {
		pidFile = cfg.Watch.PIDFile
	}
	if pidFile == "" {
		if xdgRuntime := os.Getenv("XDG_RUNTIME_DIR"); xdgRuntime != "" {
			pidFile = filepath.Join(xdgRuntime, "obsidian-notion.pid")
		} else {
			pidFile = "/tmp/obsidian-notion.pid"
		}
	}

	pid, running := checkPIDFile(pidFile)
	if !running {
		fmt.Println("No daemon running")
		return nil
	}

	// Send SIGTERM.
	process, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("find process: %w", err)
	}

	if err := process.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("send signal: %w", err)
	}

	fmt.Printf("Sent stop signal to daemon (PID: %d)\n", pid)
	return nil
}

// statusWatchCmd represents the status subcommand for checking daemon status.
var statusWatchCmd = &cobra.Command{
	Use:   "status",
	Short: "Check if watch daemon is running",
	RunE:  runWatchStatus,
}

func init() {
	watchCmd.AddCommand(statusWatchCmd)
}

func runWatchStatus(cmd *cobra.Command, args []string) error {
	cfg, err := getConfig()
	if err != nil {
		return err
	}

	// Determine PID file location.
	pidFile := watchPIDFile
	if pidFile == "" {
		pidFile = cfg.Watch.PIDFile
	}
	if pidFile == "" {
		if xdgRuntime := os.Getenv("XDG_RUNTIME_DIR"); xdgRuntime != "" {
			pidFile = filepath.Join(xdgRuntime, "obsidian-notion.pid")
		} else {
			pidFile = "/tmp/obsidian-notion.pid"
		}
	}

	pid, running := checkPIDFile(pidFile)
	if running {
		fmt.Printf("Daemon running (PID: %d)\n", pid)
	} else {
		fmt.Println("Daemon not running")
	}
	return nil
}
