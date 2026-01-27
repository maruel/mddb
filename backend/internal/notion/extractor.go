// Orchestrates extraction of Notion workspace data.

package notion

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/maruel/mddb/backend/internal/storage/content"
)

// ExtractOptions defines options for extraction.
type ExtractOptions struct {
	// Filter what to extract
	DatabaseIDs []string // specific databases, empty = all
	PageIDs     []string // specific pages, empty = all

	// Behavior
	IncludeContent bool // fetch page content (blocks)
	MaxDepth       int  // max nesting depth (0 = unlimited)

	// View manifest for importing views
	Manifest *ViewManifest
}

// Extractor orchestrates the extraction of Notion data.
type Extractor struct {
	client   *Client
	mapper   *Mapper
	writer   *Writer
	progress ProgressReporter
}

// NewExtractor creates a new extractor.
func NewExtractor(client *Client, writer *Writer, progress ProgressReporter) *Extractor {
	if progress == nil {
		progress = &NullProgress{}
	}
	return &Extractor{
		client:   client,
		mapper:   NewMapper(),
		writer:   writer,
		progress: progress,
	}
}

// databaseData holds fetched data for a database during extraction.
type databaseData struct {
	db   *Database
	node *content.Node
	rows []Page
}

// Extract performs the full extraction based on options.
func (e *Extractor) Extract(ctx context.Context, opts ExtractOptions) (*ExtractStats, error) {
	startTime := time.Now()
	stats := &ExtractStats{}

	// Ensure workspace directory exists
	if err := e.writer.EnsureWorkspace(); err != nil {
		return nil, fmt.Errorf("failed to create workspace: %w", err)
	}

	// Discover content
	databases, pages, err := e.discoverContent(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to discover content: %w", err)
	}

	total := len(databases) + len(pages)
	e.progress.OnStart(total)

	// Phase 0: Pre-assign mddb IDs to all items for parent resolution
	for i := range databases {
		e.mapper.AssignNodeID(databases[i].ID)
	}
	for i := range pages {
		e.mapper.AssignNodeID(pages[i].ID)
	}

	// Phase 1: Fetch all database rows and map databases
	dbDataList := make([]*databaseData, 0, len(databases))
	for i := range databases {
		node, err := e.mapper.MapDatabase(databases[i])
		if err != nil {
			e.progress.OnError(fmt.Errorf("database %s: failed to map: %w", databases[i].ID, err))
			stats.Errors++
			continue
		}

		rows, err := e.client.QueryDatabaseAll(ctx, databases[i].ID, nil)
		if err != nil {
			e.progress.OnError(fmt.Errorf("database %s: failed to query: %w", databases[i].ID, err))
			stats.Errors++
			continue
		}

		// Pre-assign mddb IDs to all rows
		for j := range rows {
			e.mapper.AssignRecordID(rows[j].ID)
		}

		dbDataList = append(dbDataList, &databaseData{
			db:   databases[i],
			node: node,
			rows: rows,
		})
	}

	// Phase 2: Map and write all database records
	current := 0
	for _, data := range dbDataList {
		current++
		e.progress.OnProgress(current, "Database: "+richTextToPlain(data.db.Title))

		// Apply views from manifest
		if opts.Manifest != nil {
			data.node.Views = opts.Manifest.ToContentViews(data.db.ID)
		}

		// Resolve relation target IDs in schema
		e.mapper.ResolveRelations(data.node)

		// Write node and manifest entry
		if err := e.writer.WriteNode(data.node, ""); err != nil {
			e.progress.OnError(fmt.Errorf("database %s: failed to write node: %w", data.db.ID, err))
			stats.Errors++
			continue
		}
		if err := e.writer.WriteNodeEntry(data.node); err != nil {
			e.progress.OnError(fmt.Errorf("database %s: failed to write manifest: %w", data.db.ID, err))
		}

		// Map and write records
		var records []*content.DataRecord
		for i := range data.rows {
			record, err := e.mapper.MapDatabasePage(&data.rows[i], data.db.Properties)
			if err != nil {
				e.progress.OnWarning(fmt.Sprintf("Failed to map row %s: %v", data.rows[i].ID, err))
				continue
			}
			records = append(records, record)
		}

		if err := e.writer.WriteRecords(data.node.ID, data.node.Properties, records); err != nil {
			e.progress.OnError(fmt.Errorf("database %s: failed to write records: %w", data.db.ID, err))
			stats.Errors++
			continue
		}

		stats.Databases++
		stats.Records += len(records)
	}

	// Phase 3: Extract standalone pages
	for i := range pages {
		current++
		title := extractPageTitle(&pages[i])
		e.progress.OnProgress(current, "Page: "+title)

		if err := e.extractPage(ctx, &pages[i], opts); err != nil {
			e.progress.OnError(fmt.Errorf("page %s: %w", pages[i].ID, err))
			stats.Errors++
			continue
		}
		stats.Pages++
	}

	stats.Duration = time.Since(startTime)
	e.progress.OnComplete(*stats)
	return stats, nil
}

// discoverContent finds all databases and pages to extract.
func (e *Extractor) discoverContent(ctx context.Context, opts ExtractOptions) ([]*Database, []Page, error) {
	var databases []*Database
	var pages []Page

	// If specific IDs provided, fetch those directly
	if len(opts.DatabaseIDs) > 0 {
		for _, id := range opts.DatabaseIDs {
			db, err := e.client.GetDatabase(ctx, id)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to get database %s: %w", id, err)
			}
			databases = append(databases, db)
		}
	}

	if len(opts.PageIDs) > 0 {
		for _, id := range opts.PageIDs {
			page, err := e.client.GetPage(ctx, id)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to get page %s: %w", id, err)
			}
			pages = append(pages, *page)
		}
	}

	// If no specific IDs, search for all
	if len(opts.DatabaseIDs) == 0 && len(opts.PageIDs) == 0 {
		// Search for databases
		dbResults, err := e.client.SearchAll(ctx, "", &SearchFilter{
			Value:    "database",
			Property: "object",
		})
		if err != nil {
			return nil, nil, fmt.Errorf("failed to search databases: %w", err)
		}

		for i := range dbResults {
			if dbResults[i].Object == "database" {
				db, err := e.client.GetDatabase(ctx, dbResults[i].ID)
				if err != nil {
					e.progress.OnWarning(fmt.Sprintf("Failed to get database %s: %v", dbResults[i].ID, err))
					continue
				}
				databases = append(databases, db)
			}
		}

		// Search for pages (only standalone pages, not database rows)
		pageResults, err := e.client.SearchAll(ctx, "", &SearchFilter{
			Value:    "page",
			Property: "object",
		})
		if err != nil {
			return nil, nil, fmt.Errorf("failed to search pages: %w", err)
		}

		for i := range pageResults {
			if pageResults[i].Object == "page" {
				// Skip pages that are database rows
				if pageResults[i].Parent.Type == "database_id" {
					continue
				}

				page, err := e.client.GetPage(ctx, pageResults[i].ID)
				if err != nil {
					e.progress.OnWarning(fmt.Sprintf("Failed to get page %s: %v", pageResults[i].ID, err))
					continue
				}
				pages = append(pages, *page)
			}
		}
	}

	return databases, pages, nil
}

// extractPage extracts a standalone page.
func (e *Extractor) extractPage(ctx context.Context, page *Page, opts ExtractOptions) error {
	// Map page to node
	node, err := e.mapper.MapPage(page)
	if err != nil {
		return fmt.Errorf("failed to map page: %w", err)
	}

	// Get page content if requested
	var markdown string
	if opts.IncludeContent {
		blocks, err := e.client.GetBlockChildrenRecursive(ctx, page.ID, opts.MaxDepth)
		if err != nil {
			e.progress.OnWarning(fmt.Sprintf("Failed to get blocks for %s: %v", page.ID, err))
		} else {
			markdown = BlocksToMarkdown(blocks)
		}
	}

	// Write node and manifest entry
	if err := e.writer.WriteNode(node, markdown); err != nil {
		return fmt.Errorf("failed to write node: %w", err)
	}
	if err := e.writer.WriteNodeEntry(node); err != nil {
		return fmt.Errorf("failed to write manifest: %w", err)
	}

	return nil
}

// DryRunResult contains items that would be extracted during a dry run.
type DryRunResult struct {
	Databases []DryRunItem `json:"databases"`
	Pages     []DryRunItem `json:"pages"`
}

// DryRunItem represents an item that would be extracted.
type DryRunItem struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Type  string `json:"type"`
}

// DryRun discovers content without extracting it.
func (e *Extractor) DryRun(ctx context.Context, opts ExtractOptions) (*DryRunResult, error) {
	databases, pages, err := e.discoverContent(ctx, opts)
	if err != nil {
		return nil, err
	}

	result := &DryRunResult{}

	for i := range databases {
		result.Databases = append(result.Databases, DryRunItem{
			ID:    databases[i].ID,
			Title: richTextToPlain(databases[i].Title),
			Type:  "database",
		})
	}

	for i := range pages {
		result.Pages = append(result.Pages, DryRunItem{
			ID:    pages[i].ID,
			Title: extractPageTitle(&pages[i]),
			Type:  "page",
		})
	}

	return result, nil
}

// DryRunJSON returns the dry run result as JSON.
func (e *Extractor) DryRunJSON(ctx context.Context, opts ExtractOptions) (string, error) {
	result, err := e.DryRun(ctx, opts)
	if err != nil {
		return "", err
	}

	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", err
	}

	return string(data), nil
}
