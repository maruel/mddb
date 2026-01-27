// Defines progress reporting interfaces and implementations.

package notion

import (
	"fmt"
	"io"
	"time"
)

// ExtractStats contains statistics about an extraction operation.
type ExtractStats struct {
	Pages     int           `json:"pages"`
	Databases int           `json:"databases"`
	Records   int           `json:"records"`
	Assets    int           `json:"assets"`
	Errors    int           `json:"errors"`
	Duration  time.Duration `json:"duration"`
}

// ProgressReporter is the interface for reporting extraction progress.
type ProgressReporter interface {
	OnStart(total int)
	OnProgress(current int, item string)
	OnWarning(msg string)
	OnError(err error)
	OnComplete(stats ExtractStats)
}

// CLIProgress writes progress to stdout/stderr.
type CLIProgress struct {
	Out io.Writer
	Err io.Writer
}

// OnStart is called when extraction begins.
func (p *CLIProgress) OnStart(total int) {
	_, _ = fmt.Fprintf(p.Out, "Found %d items to import\n\n", total)
}

// OnProgress is called for each item processed.
func (p *CLIProgress) OnProgress(current int, item string) {
	_, _ = fmt.Fprintf(p.Out, "[%d] %s\n", current, item)
}

// OnWarning is called for non-fatal issues.
func (p *CLIProgress) OnWarning(msg string) {
	_, _ = fmt.Fprintf(p.Err, "Warning: %s\n", msg)
}

// OnError is called for errors during extraction.
func (p *CLIProgress) OnError(err error) {
	_, _ = fmt.Fprintf(p.Err, "Error: %v\n", err)
}

// OnComplete is called when extraction finishes.
func (p *CLIProgress) OnComplete(stats ExtractStats) {
	_, _ = fmt.Fprintf(p.Out, "\nComplete!\n")
	_, _ = fmt.Fprintf(p.Out, "---------\n")
	_, _ = fmt.Fprintf(p.Out, "Databases: %d\n", stats.Databases)
	_, _ = fmt.Fprintf(p.Out, "Pages:     %d\n", stats.Pages)
	_, _ = fmt.Fprintf(p.Out, "Records:   %d\n", stats.Records)
	_, _ = fmt.Fprintf(p.Out, "Assets:    %d\n", stats.Assets)
	if stats.Errors > 0 {
		_, _ = fmt.Fprintf(p.Out, "Errors:    %d\n", stats.Errors)
	}
	_, _ = fmt.Fprintf(p.Out, "Duration:  %s\n", stats.Duration.Round(time.Second))
}

// ProgressUpdate represents a progress update for channel-based reporting.
type ProgressUpdate struct {
	Type    string        `json:"type"` // "start", "progress", "warning", "error", "complete"
	Current int           `json:"current,omitempty"`
	Total   int           `json:"total,omitempty"`
	Message string        `json:"message,omitempty"`
	Stats   *ExtractStats `json:"stats,omitempty"`
}

// ChannelProgress sends progress updates via a channel.
type ChannelProgress struct {
	Updates chan<- ProgressUpdate
	total   int
}

// NewChannelProgress creates a new channel-based progress reporter.
func NewChannelProgress(updates chan<- ProgressUpdate) *ChannelProgress {
	return &ChannelProgress{Updates: updates}
}

// OnStart is called when extraction begins.
func (p *ChannelProgress) OnStart(total int) {
	p.total = total
	p.Updates <- ProgressUpdate{Type: "start", Total: total}
}

// OnProgress is called for each item processed.
func (p *ChannelProgress) OnProgress(current int, item string) {
	p.Updates <- ProgressUpdate{Type: "progress", Current: current, Total: p.total, Message: item}
}

// OnWarning is called for non-fatal issues.
func (p *ChannelProgress) OnWarning(msg string) {
	p.Updates <- ProgressUpdate{Type: "warning", Message: msg}
}

// OnError is called for errors during extraction.
func (p *ChannelProgress) OnError(err error) {
	p.Updates <- ProgressUpdate{Type: "error", Message: err.Error()}
}

// OnComplete is called when extraction finishes.
func (p *ChannelProgress) OnComplete(stats ExtractStats) {
	p.Updates <- ProgressUpdate{Type: "complete", Stats: &stats}
}

// NullProgress discards all progress updates.
type NullProgress struct{}

// OnStart is called when extraction begins.
func (p *NullProgress) OnStart(total int) {}

// OnProgress is called for each item processed.
func (p *NullProgress) OnProgress(current int, item string) {}

// OnWarning is called for non-fatal issues.
func (p *NullProgress) OnWarning(msg string) {}

// OnError is called for errors during extraction.
func (p *NullProgress) OnError(err error) {}

// OnComplete is called when extraction finishes.
func (p *NullProgress) OnComplete(stats ExtractStats) {}
