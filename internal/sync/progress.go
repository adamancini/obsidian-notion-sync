package sync

import (
	"fmt"
	"io"
	"strings"
	"sync"
	"time"
)

// Progress tracks and displays sync progress.
type Progress struct {
	total     int
	completed int
	failed    int
	startTime time.Time
	writer    io.Writer
	mu        sync.Mutex
	lastPrint time.Time
	barWidth  int
	enabled   bool
}

// NewProgress creates a new progress tracker.
func NewProgress(total int, writer io.Writer) *Progress {
	return &Progress{
		total:     total,
		startTime: time.Now(),
		writer:    writer,
		barWidth:  40,
		enabled:   true,
	}
}

// SetEnabled enables or disables progress output.
func (p *Progress) SetEnabled(enabled bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.enabled = enabled
}

// Increment adds to the completed count.
func (p *Progress) Increment() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.completed++
	p.render()
}

// IncrementFailed adds to the failed count.
func (p *Progress) IncrementFailed() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.completed++
	p.failed++
	p.render()
}

// Update sets the current progress values.
func (p *Progress) Update(completed, failed int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.completed = completed
	p.failed = failed
	p.render()
}

// render outputs the progress bar (throttled to avoid flicker).
func (p *Progress) render() {
	if !p.enabled {
		return
	}

	// Throttle updates to every 100ms.
	if time.Since(p.lastPrint) < 100*time.Millisecond && p.completed < p.total {
		return
	}
	p.lastPrint = time.Now()

	// Calculate progress percentage.
	percent := float64(p.completed) / float64(p.total) * 100
	if p.total == 0 {
		percent = 100
	}

	// Build progress bar.
	filled := int(float64(p.barWidth) * float64(p.completed) / float64(p.total))
	if p.total == 0 {
		filled = p.barWidth
	}
	bar := strings.Repeat("█", filled) + strings.Repeat("░", p.barWidth-filled)

	// Calculate ETA.
	elapsed := time.Since(p.startTime)
	eta := ""
	if p.completed > 0 && p.completed < p.total {
		remaining := time.Duration(float64(elapsed) / float64(p.completed) * float64(p.total-p.completed))
		eta = fmt.Sprintf(" ETA: %s", formatDuration(remaining))
	}

	// Build status line.
	status := fmt.Sprintf("\r[%s] %3.0f%% (%d/%d)", bar, percent, p.completed, p.total)
	if p.failed > 0 {
		status += fmt.Sprintf(" [%d failed]", p.failed)
	}
	status += eta

	// Clear to end of line and print.
	fmt.Fprintf(p.writer, "%s\033[K", status)
}

// Finish completes the progress and prints final stats.
func (p *Progress) Finish() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.enabled {
		return
	}

	elapsed := time.Since(p.startTime)

	// Clear the progress line.
	fmt.Fprintf(p.writer, "\r\033[K")

	// Print summary.
	rate := float64(p.completed) / elapsed.Seconds()
	if elapsed.Seconds() < 0.001 {
		rate = float64(p.completed)
	}

	fmt.Fprintf(p.writer, "Processed %d items in %s (%.1f/sec)\n",
		p.completed, formatDuration(elapsed), rate)
}

// formatDuration formats a duration in a human-readable way.
func formatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm%ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%dh%dm", int(d.Hours()), int(d.Minutes())%60)
}

// SimpleCallback returns a progress callback function for use with ProcessWithProgress.
func (p *Progress) SimpleCallback() func(completed, total int) {
	return func(completed, total int) {
		p.mu.Lock()
		p.completed = completed
		p.render()
		p.mu.Unlock()
	}
}
