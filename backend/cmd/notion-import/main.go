// Package main is the entry point for the notion-import CLI tool.
//
// notion-import extracts data from Notion workspaces and imports it into mddb
// storage format. It supports extracting pages, databases, and records with
// rate limiting to respect Notion's API limits.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/maruel/mddb/backend/internal/notion"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "notion-import: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	// Flags
	token := flag.String("token", "", "Notion integration token (required, or set NOTION_TOKEN)")
	workspaceID := flag.String("workspace", "", "Target mddb workspace ID (required)")
	outputDir := flag.String("output", "./data", "Output directory")
	databaseIDs := flag.String("database", "", "Comma-separated database IDs to import (default: all)")
	pageIDs := flag.String("page", "", "Comma-separated page IDs to import (default: all)")
	includeContent := flag.Bool("include-content", true, "Fetch page content (blocks)")
	maxDepth := flag.Int("max-depth", 0, "Max nesting depth for blocks (0=unlimited)")
	dryRun := flag.Bool("dry-run", false, "Show what would be imported without importing")
	flag.Parse()

	// Validate required flags
	if *token == "" {
		*token = os.Getenv("NOTION_TOKEN")
	}
	if *token == "" {
		return errors.New("--token or NOTION_TOKEN environment variable is required")
	}

	if *workspaceID == "" && !*dryRun {
		return errors.New("--workspace is required")
	}

	// Parse multi-value flags
	var dbIDs, pgIDs []string
	if *databaseIDs != "" {
		dbIDs = strings.Split(*databaseIDs, ",")
		for i := range dbIDs {
			dbIDs[i] = strings.TrimSpace(dbIDs[i])
		}
	}
	if *pageIDs != "" {
		pgIDs = strings.Split(*pageIDs, ",")
		for i := range pgIDs {
			pgIDs[i] = strings.TrimSpace(pgIDs[i])
		}
	}

	// Setup context with signal handling
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	defer stop()

	// Create client and extractor
	client := notion.NewClient(*token)
	writer := notion.NewWriter(*outputDir, *workspaceID)
	progress := &notion.CLIProgress{
		Out: os.Stdout,
		Err: os.Stderr,
	}
	extractor := notion.NewExtractor(client, writer, progress)

	opts := notion.ExtractOptions{
		DatabaseIDs:    dbIDs,
		PageIDs:        pgIDs,
		IncludeContent: *includeContent,
		MaxDepth:       *maxDepth,
	}

	// Print header
	fmt.Println("Notion Import")
	fmt.Println("=============")
	fmt.Println("Connecting to Notion API...")
	fmt.Println()

	// Dry run or full extraction
	if *dryRun {
		json, err := extractor.DryRunJSON(ctx, opts)
		if err != nil {
			return fmt.Errorf("dry run failed: %w", err)
		}
		fmt.Println(json)
		return nil
	}

	// Full extraction
	stats, err := extractor.Extract(ctx, opts)
	if err != nil {
		return fmt.Errorf("extraction failed: %w", err)
	}

	fmt.Printf("\nOutput: %s/%s/\n", *outputDir, *workspaceID)

	if stats.Errors > 0 {
		return fmt.Errorf("%d errors occurred during import", stats.Errors)
	}

	return nil
}
