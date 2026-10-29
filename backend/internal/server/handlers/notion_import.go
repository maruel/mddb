// Handles Notion import endpoints for creating workspaces from Notion data.

package handlers

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/maruel/ksid"
	"github.com/maruel/mddb/backend/internal/notion"
	"github.com/maruel/mddb/backend/internal/server/dto"
	"github.com/maruel/mddb/backend/internal/storage/identity"
)

// importState tracks the state of a running or completed import.
type importState struct {
	mu        sync.Mutex
	status    string // idle, running, completed, failed, cancelled
	progress  int
	total     int
	message   string
	stats     *notion.ExtractStats
	cancel    context.CancelFunc
	startTime time.Time
}

type importJob struct {
	wsID   ksid.ID
	token  string
	state  *importState
	cancel context.CancelFunc
}

// NotionImportHandler handles Notion import requests.
type NotionImportHandler struct {
	Svc *Services
	Cfg *Config

	mu     sync.Mutex
	states map[ksid.ID]*importState // wsID -> state
}

// NewNotionImportHandler creates a new handler for Notion imports.
func NewNotionImportHandler(svc *Services, cfg *Config) *NotionImportHandler {
	return &NotionImportHandler{
		Svc:    svc,
		Cfg:    cfg,
		states: make(map[ksid.ID]*importState),
	}
}

// StartImport creates a new workspace and starts an async Notion import.
func (h *NotionImportHandler) StartImport(ctx context.Context, orgID ksid.ID, user *identity.User, req *dto.NotionImportRequest) (*dto.NotionImportResponse, error) {
	// Check server-wide workspace quota
	if h.Cfg.Quotas.MaxWorkspaces > 0 && h.Svc.Workspace.Count() >= h.Cfg.Quotas.MaxWorkspaces {
		return nil, dto.QuotaExceeded("workspaces", h.Cfg.Quotas.MaxWorkspaces)
	}

	// Validate token and get workspace name from Notion
	client := notion.NewClient(req.NotionToken)
	workspaceName, err := client.GetWorkspaceName(ctx)
	if err != nil {
		return nil, dto.InvalidField("notion_token", "invalid Notion token: "+err.Error())
	}
	if workspaceName == "" {
		workspaceName = "Notion Import"
	}

	// Reserve a worker before publishing a workspace. The worker waits for setup
	// to finish, so shutdown cannot reject the import after membership is visible.
	jobs := make(chan importJob, 1)
	importCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	if !h.Svc.StartBackground(importCtx, 24*time.Hour, func(ctx context.Context) {
		job, ok := <-jobs
		if ok {
			defer job.cancel()
			h.runImport(ctx, job.wsID, job.token, job.state)
		}
	}) {
		cancel()
		return nil, dto.InternalWithError("Failed to start import", context.Canceled)
	}
	handedOff := false
	defer func() {
		if !handedOff {
			cancel()
		}
		close(jobs)
	}()

	// Create workspace
	ws, err := h.Svc.Workspace.Create(ctx, orgID, workspaceName)
	if err != nil {
		return nil, dto.InternalWithError("Failed to create workspace", err)
	}

	// Initialize storage before publishing membership.
	if err := h.Svc.FileStore.InitWorkspace(ctx, ws.ID); err != nil {
		err = errors.Join(err, h.Svc.Workspace.Delete(ws.ID), h.Svc.FileStore.DiscardWorkspace(ws.ID))
		return nil, dto.InternalWithError("Failed to initialize workspace storage", err)
	}
	if _, err := h.Svc.WSMembership.Create(user.ID, ws.ID, identity.WSRoleAdmin); err != nil {
		err = errors.Join(err, h.Svc.Workspace.Delete(ws.ID), h.Svc.FileStore.DiscardWorkspace(ws.ID))
		return nil, dto.InternalWithError("Failed to create workspace membership", err)
	}

	// Create import state with a context that outlives the HTTP request but preserves values.
	state := &importState{
		status:    "running",
		cancel:    cancel,
		startTime: time.Now(),
	}

	h.mu.Lock()
	h.states[ws.ID] = state
	h.mu.Unlock()

	jobs <- importJob{wsID: ws.ID, token: req.NotionToken, state: state, cancel: cancel}
	handedOff = true

	return &dto.NotionImportResponse{
		WorkspaceID:   ws.ID,
		WorkspaceName: ws.Name,
		Status:        "running",
	}, nil
}

// GetStatus returns the current status of an import.
func (h *NotionImportHandler) GetStatus(_ context.Context, orgID ksid.ID, _ *identity.User, req *dto.NotionImportStatusRequest) (*dto.NotionImportStatusResponse, error) {
	// Verify workspace belongs to org
	ws, err := h.Svc.Workspace.Get(req.ImportWsID)
	if err != nil {
		return nil, dto.NotFound("workspace")
	}
	if ws.OrganizationID != orgID {
		return nil, dto.Forbidden("Workspace does not belong to this organization")
	}

	h.mu.Lock()
	state := h.states[req.ImportWsID]
	h.mu.Unlock()

	if state == nil {
		// No import in progress or completed
		return &dto.NotionImportStatusResponse{
			Status: "idle",
		}, nil
	}

	state.mu.Lock()
	defer state.mu.Unlock()

	resp := &dto.NotionImportStatusResponse{
		Status:   state.status,
		Progress: state.progress,
		Total:    state.total,
		Message:  state.message,
	}

	if state.stats != nil {
		resp.Pages = state.stats.Pages
		resp.Databases = state.stats.Databases
		resp.Records = state.stats.Records
		resp.Assets = state.stats.Assets
		resp.Errors = state.stats.Errors
		resp.DurationMs = state.stats.Duration.Milliseconds()
	} else if state.status == "running" {
		resp.DurationMs = time.Since(state.startTime).Milliseconds()
	}

	return resp, nil
}

// CancelImport cancels a running import.
func (h *NotionImportHandler) CancelImport(_ context.Context, wsID ksid.ID, _ *identity.User, _ *dto.NotionImportCancelRequest) (*dto.NotionImportCancelResponse, error) {
	h.mu.Lock()
	state := h.states[wsID]
	h.mu.Unlock()

	if state == nil {
		return nil, dto.NotFound("import")
	}

	state.mu.Lock()
	if state.status != "running" {
		state.mu.Unlock()
		return nil, dto.InvalidField("status", "import is not running")
	}
	state.status = "cancelled"
	state.message = "Import cancelled by user"
	state.mu.Unlock()

	// Cancel the context to stop the import goroutine
	if state.cancel != nil {
		state.cancel()
	}

	return &dto.NotionImportCancelResponse{Ok: true}, nil
}

// runImport performs the actual Notion import in the background.
func (h *NotionImportHandler) runImport(ctx context.Context, wsID ksid.ID, notionToken string, state *importState) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("Notion import panic", "wsID", wsID, "err", r)
			state.mu.Lock()
			state.status = "failed"
			state.message = "Internal error during import"
			state.mu.Unlock()
		}
	}()

	// Create Notion client
	client := notion.NewClient(notionToken)

	// Create writer for workspace
	rootDir := h.Svc.FileStore.RootDir()
	writer := notion.NewWriter(rootDir, wsID.String())

	// Create progress reporter that updates state
	progress := &stateProgressReporter{state: state}

	// Create extractor
	extractor := notion.NewExtractor(client, writer, progress)

	// Run extraction
	opts := notion.ExtractOptions{
		IncludeContent: true,
		MaxDepth:       0, // unlimited
	}

	stats, err := extractor.Extract(ctx, opts)

	state.mu.Lock()
	defer state.mu.Unlock()

	// Check if cancelled
	if state.status == "cancelled" {
		return
	}

	if err != nil {
		switch ctx.Err() {
		case context.DeadlineExceeded:
			state.status = "failed"
			state.message = "Import exceeded the 24-hour time limit"
		case context.Canceled:
			state.status = "cancelled"
			state.message = "Import cancelled"
		default:
			state.status = "failed"
			state.message = err.Error()
		}
		slog.Error("Notion import failed", "wsID", wsID, "err", err)
		return
	}

	state.status = "completed"
	state.stats = stats
	state.message = "Import completed successfully"
	slog.Info("Notion import completed", "wsID", wsID, "pages", stats.Pages, "databases", stats.Databases)
}

// stateProgressReporter implements notion.ProgressReporter to update import state.
type stateProgressReporter struct {
	state *importState
}

func (p *stateProgressReporter) OnStart(total int) {
	p.state.mu.Lock()
	defer p.state.mu.Unlock()
	p.state.total = total
	p.state.message = "Starting import..."
}

func (p *stateProgressReporter) OnProgress(current int, item string) {
	p.state.mu.Lock()
	defer p.state.mu.Unlock()
	p.state.progress = current
	p.state.message = item
}

func (p *stateProgressReporter) OnWarning(msg string) {
	// Warnings are logged but don't update user-visible message
	slog.Warn("Notion import warning", "msg", msg)
}

func (p *stateProgressReporter) OnError(err error) {
	slog.Error("Notion import error", "err", err)
}

func (p *stateProgressReporter) OnComplete(stats notion.ExtractStats) {
	p.state.mu.Lock()
	defer p.state.mu.Unlock()
	p.state.stats = &stats
}
