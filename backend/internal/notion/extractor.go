// Orchestrates extraction of Notion workspace data.

package notion

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/maruel/mddb/backend/internal/ksid"
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
	assets   *AssetDownloader
	imported map[string]bool // Track already-imported Notion IDs
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

	// Load existing ID mapping for incremental imports
	existingIDs, err := e.writer.LoadIDMapping()
	if err != nil {
		e.progress.OnWarning(fmt.Sprintf("Failed to load ID mapping, starting fresh: %v", err))
		existingIDs = make(map[string]ksid.ID)
	}
	if len(existingIDs) > 0 {
		e.progress.OnProgress(0, fmt.Sprintf("Loaded %d existing ID mappings", len(existingIDs)))
		e.mapper = NewMapperWithIDs(existingIDs)
	}

	// Clear nodes manifest for fresh import (IDs are preserved via mapping)
	if err := e.writer.ClearNodesManifest(); err != nil {
		e.progress.OnWarning(fmt.Sprintf("Failed to clear nodes manifest: %v", err))
	}

	// Create asset downloader and import tracker
	e.assets = NewAssetDownloader(e.writer.workspacePath())
	e.imported = make(map[string]bool)

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

		// Download icon and cover
		e.mapper.MapDatabaseIconCover(node, databases[i], e.assets)

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

		// Mark as imported to prevent duplicate extraction from child blocks
		e.imported[data.db.ID] = true

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

		// Map and write records (set asset context for file downloads)
		e.mapper.SetAssetContext(e.assets, data.node.ID)
		var records []*content.DataRecord
		for i := range data.rows {
			record, err := e.mapper.MapDatabasePage(&data.rows[i], data.db.Properties)
			if err != nil {
				e.progress.OnWarning(fmt.Sprintf("Failed to map row %s: %v", data.rows[i].ID, err))
				continue
			}
			records = append(records, record)
		}

		// Clear existing data for re-import (IDs preserved via mapping)
		if err := e.writer.ClearNodeData(data.node.ID); err != nil {
			e.progress.OnWarning(fmt.Sprintf("Failed to clear existing data: %v", err))
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

	// Gather asset stats
	if e.assets != nil {
		stats.Assets = e.assets.Downloaded
	}

	// Save ID mapping for future incremental imports
	if err := e.writer.SaveIDMapping(e.mapper.NotionToMddb); err != nil {
		e.progress.OnWarning(fmt.Sprintf("Failed to save ID mapping: %v", err))
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
	// Skip if already imported
	if e.imported[page.ID] {
		return nil
	}
	e.imported[page.ID] = true

	// Map page to node
	node, err := e.mapper.MapPage(page)
	if err != nil {
		return fmt.Errorf("failed to map page: %w", err)
	}

	// Download icon and cover
	e.mapper.MapPageIconCover(node, page, e.assets)

	// Get page content if requested
	var markdown string
	var childRefs []ChildRef
	if opts.IncludeContent {
		blocks, err := e.client.GetBlockChildrenRecursive(ctx, page.ID, opts.MaxDepth)
		if err != nil {
			e.progress.OnWarning(fmt.Sprintf("Failed to get blocks for %s: %v", page.ID, err))
		} else {
			// Collect child page/database references
			childRefs = collectChildRefs(blocks)

			// Pre-assign IDs to children so we can link to them
			for _, ref := range childRefs {
				e.mapper.AssignNodeID(ref.ID)
			}

			// Use markdown converter with asset downloading and child links
			converter := NewMarkdownConverterWithLinks(e.assets, node.ID, e.mapper)
			markdown = converter.Convert(blocks)
		}
	}

	// Write node and manifest entry
	if err := e.writer.WriteNode(node, markdown); err != nil {
		return fmt.Errorf("failed to write node: %w", err)
	}
	if err := e.writer.WriteNodeEntry(node); err != nil {
		return fmt.Errorf("failed to write manifest: %w", err)
	}

	// Import child pages and databases
	for _, ref := range childRefs {
		if ref.Type == "page" {
			childPage, err := e.client.GetPage(ctx, ref.ID)
			if err != nil {
				e.progress.OnWarning(fmt.Sprintf("Failed to get child page %s: %v", ref.ID, err))
				continue
			}
			if err := e.extractPage(ctx, childPage, opts); err != nil {
				e.progress.OnWarning(fmt.Sprintf("Failed to extract child page %s: %v", ref.ID, err))
			}
		} else if ref.Type == "database" {
			db, err := e.client.GetDatabase(ctx, ref.ID)
			if err != nil {
				e.progress.OnWarning(fmt.Sprintf("Failed to get child database %s: %v", ref.ID, err))
				continue
			}
			if err := e.extractDatabase(ctx, db, opts); err != nil {
				e.progress.OnWarning(fmt.Sprintf("Failed to extract child database %s: %v", ref.ID, err))
			}
		}
	}

	return nil
}

// extractDatabase extracts a single database and its records.
func (e *Extractor) extractDatabase(ctx context.Context, db *Database, opts ExtractOptions) error {
	// Skip if already imported
	if e.imported[db.ID] {
		return nil
	}
	e.imported[db.ID] = true

	node, err := e.mapper.MapDatabase(db)
	if err != nil {
		return fmt.Errorf("failed to map database: %w", err)
	}

	// Download icon and cover
	e.mapper.MapDatabaseIconCover(node, db, e.assets)

	// Clear pending relations for this database
	e.mapper.ClearPendingRelations()

	rows, err := e.client.QueryDatabaseAll(ctx, db.ID, nil)
	if err != nil {
		return fmt.Errorf("failed to query database: %w", err)
	}

	// Pre-assign mddb IDs to all rows
	for i := range rows {
		e.mapper.AssignRecordID(rows[i].ID)
	}

	// Apply views from manifest
	if opts.Manifest != nil {
		node.Views = opts.Manifest.ToContentViews(db.ID)
	}

	// Resolve relation target IDs in schema
	e.mapper.ResolveRelations(node)

	// Write node and manifest entry
	if err := e.writer.WriteNode(node, ""); err != nil {
		return fmt.Errorf("failed to write node: %w", err)
	}
	if err := e.writer.WriteNodeEntry(node); err != nil {
		return fmt.Errorf("failed to write manifest: %w", err)
	}

	// Map and write records (set asset context for file downloads)
	e.mapper.SetAssetContext(e.assets, node.ID)
	var records []*content.DataRecord
	for i := range rows {
		record, err := e.mapper.MapDatabasePage(&rows[i], db.Properties)
		if err != nil {
			e.progress.OnWarning(fmt.Sprintf("Failed to map row %s: %v", rows[i].ID, err))
			continue
		}
		records = append(records, record)
	}

	// Clear existing data for re-import (IDs preserved via mapping)
	if err := e.writer.ClearNodeData(node.ID); err != nil {
		e.progress.OnWarning(fmt.Sprintf("Failed to clear existing data: %v", err))
	}
	if err := e.writer.WriteRecords(node.ID, node.Properties, records); err != nil {
		return fmt.Errorf("failed to write records: %w", err)
	}

	return nil
}

// ChildRef represents a reference to a child page or database found in block content.
type ChildRef struct {
	ID    string
	Type  string // "page" or "database"
	Title string
}

// collectChildRefs extracts child page and database references from blocks.
func collectChildRefs(blocks []Block) []ChildRef {
	var refs []ChildRef
	for i := range blocks {
		block := &blocks[i]
		switch block.Type {
		case "child_page":
			if block.ChildPage != nil {
				refs = append(refs, ChildRef{
					ID:    block.ID,
					Type:  "page",
					Title: block.ChildPage.Title,
				})
			}
		case "child_database":
			if block.ChildDatabase != nil {
				refs = append(refs, ChildRef{
					ID:    block.ID,
					Type:  "database",
					Title: block.ChildDatabase.Title,
				})
			}
		}
		// Recurse into children
		if len(block.Children) > 0 {
			refs = append(refs, collectChildRefs(block.Children)...)
		}
	}
	return refs
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
